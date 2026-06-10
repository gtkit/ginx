// Package ginx 提供与业务无关的通用 gin 请求参数处理能力：解决 gin 请求 body 只能读取一次的痛点
// （读取后回填，使后续 ShouldBind 仍可读）、按 Content-Type 解析 body、单值 header 校验、只绑 body
// 不混入 query，以及用 http.MaxBytesReader 对 body 设硬上限等。
//
// 这些函数不含任何业务语义，按请求在 gin.Context 上缓存解析结果，适合作为各服务的通用请求处理底座。
//
// 支持的 body 类型为 application/json 与 application/x-www-form-urlencoded；不支持
// multipart/form-data（通常携带文件、体积大，与按 MaxBodyBytes 轻量探取的定位冲突）。
//
// 并发安全：与 gin.Context 本身一致。本包函数会替换 c.Request.Body、临时修改 URL.RawQuery，
// 必须在处理该请求的 handler goroutine 内调用，不得跨 goroutine 并发操作同一个 Context。
package ginx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	gtkitjson "github.com/gtkit/json"

	"github.com/gin-gonic/gin"
)

// MaxBodyBytes 是 ParseBody 允许读取并解析的请求 body 软上限，超过则视为不可解析、返回空结果。
// 它只约束本包的解析行为；如需对整个请求生命周期施加硬上限，请用 LimitRequestBody。
const MaxBodyBytes = 8 * 1024

// ctxKeyBodyParseCache 是 body 解析结果在 gin.Context 上的缓存键，使同一请求内只解析一次 body。
const ctxKeyBodyParseCache = "ginx.body_parse_cache"

var (
	// ErrDuplicateHeader 表示同一 header 出现了多个非空值。
	ErrDuplicateHeader = errors.New("header provided multiple times")
	// ErrInvalidHeaderValue 表示 header 的值非法（如含逗号的多值形式）。
	ErrInvalidHeaderValue = errors.New("header contains invalid value")
	// ErrInvalidBindContext 表示 BindBody 收到的 context / Request / URL 为 nil，无法绑定。
	ErrInvalidBindContext = errors.New("invalid bind body context")
	// ErrNoBody 表示请求没有可解析的 body：context 或 body 为 nil，或方法为 GET/HEAD。
	ErrNoBody = errors.New("request has no parsable body")
	// ErrBodyTooLarge 表示请求 body 长度超过 MaxBodyBytes 软上限，ParseBody 拒绝解析。
	ErrBodyTooLarge = errors.New("request body exceeds MaxBodyBytes")
	// ErrUnsupportedContentType 表示 Content-Type 不在 ParseBody 支持的类型范围内。
	ErrUnsupportedContentType = errors.New("unsupported content type")
	// ErrMalformedBody 表示 body 语法非法：JSON 语法错误、JSON 尾部存在多余数据或 form 编码非法，
	// 底层解析错误可经 errors.As / Unwrap 获取。
	ErrMalformedBody = errors.New("malformed request body")
)

// BodySources 承载一次请求 body 的解析结果：Available 表示是否解析成功，JSON 与 Form 分别对应
// application/json 与 application/x-www-form-urlencoded 两种 body 的解析产物（另一种为 nil）。
// JSON 中的数字以 json.Number 承载（UseNumber 解码），原样保留 body 中的字面量。
//
// Err 在 Available 为 false 时说明失败原因，可用 errors.Is 判定 ErrNoBody、ErrBodyTooLarge、
// ErrUnsupportedContentType、ErrMalformedBody（body 读取失败时为相应读取错误的包装）；
// Available 为 true 时恒为 nil。结果按请求缓存，同一请求内重复调用返回相同的 Err。
type BodySources struct {
	Available bool
	JSON      map[string]any
	Form      url.Values
	Err       error
}

// ParseBody 解析请求 body 并按请求缓存结果（同一请求内多次调用只读取/解析一次 body）。命中缓存直接
// 返回；否则经跳过判断（nil body、GET/HEAD、ContentLength 超 MaxBodyBytes）、限长读取并回填 body 后
// 按 Content-Type 解析。任一环节失败或不支持的类型均返回 Available 为 false 的结果，
// 失败原因经 Err 字段透出（见 BodySources）。
func ParseBody(c *gin.Context) (sources BodySources) {
	if c == nil {
		return BodySources{Err: ErrNoBody}
	}
	if v, ok := c.Get(ctxKeyBodyParseCache); ok {
		if cached, typeOK := v.(BodySources); typeOK {
			return cached
		}
	}
	defer func() {
		c.Set(ctxKeyBodyParseCache, sources)
	}()

	if err := skipBodyParse(c); err != nil {
		return BodySources{Err: err}
	}

	rawBody, err := readAndRestoreBody(c)
	if err != nil {
		return BodySources{Err: err}
	}
	if len(rawBody) > MaxBodyBytes {
		return BodySources{Err: ErrBodyTooLarge}
	}

	return parseRawBodySources(c.ContentType(), rawBody)
}

// BodyString 从请求 body 中读取指定字段并转为字符串：JSON body 支持 string/bool/number/Stringer
// 等标量类型，form body 取对应键值；body 不可解析时返回空串。
func BodyString(c *gin.Context, field string) string {
	sources := ParseBody(c)
	if !sources.Available {
		return ""
	}
	if v := jsonScalarString(sources.JSON[field]); v != "" {
		return v
	}
	return strings.TrimSpace(sources.Form.Get(field))
}

// jsonScalarString 将 JSON 解码出的标量值（string/bool/fmt.Stringer/float64）转为去除首尾空白的
// 字符串；非标量或不支持的类型返回空串。数字经 UseNumber 解码为 json.Number（fmt.Stringer），
// 原样返回 body 中的字面量；float64 分支仅作为后端不支持 UseNumber 时的防御。
func jsonScalarString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

// BindBody 仅将请求 body 绑定到 obj。绑定前临时清空 URL.RawQuery，避免 gin 在 form 模式下把
// query 参数一并并入绑定结果——确保只信任 body、不信任 query；绑定结束后恢复 RawQuery。
// context、Request 或 URL 为 nil 时返回 ErrInvalidBindContext。
//
// 绑定会消费 body 流（一次性语义）：同一请求内第二次调用将读到空 body。如需先探取字段再绑定，
// 先调用 ParseBody（它会回填 body）、再调用本函数。
func BindBody(c *gin.Context, obj any) error {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ErrInvalidBindContext
	}
	// 临时清空 RawQuery，防止 ShouldBind 在 form 模式下把 URL query 字段并入 body 绑定。只信 body，不信 query
	rawQuery := c.Request.URL.RawQuery
	c.Request.URL.RawQuery = ""
	defer func() { c.Request.URL.RawQuery = rawQuery }()

	if err := c.ShouldBind(obj); err != nil {
		return fmt.Errorf("bind body: %w", err)
	}
	return nil
}

// LimitRequestBody 用 http.MaxBytesReader 为请求 body 设置硬上限 maxBytes：调用后，任何对 body
// 的读取（包括下游 c.ShouldBind / BindBody）一旦累计超过 maxBytes，都会立即返回 *http.MaxBytesError
// 并停止继续读入内存，从根上防御超大 body 撑爆内存，必要时还会关闭连接。
// 这与软上限 MaxBodyBytes 不同：MaxBodyBytes 仅用于本包解析时按长度跳过、不约束下游读取，而本函数的
// 限制对整个请求生命周期生效。应在读取或绑定 body 之前调用（如路由进入处或 handler 开头）。
// maxBytes <= 0 或无 body 时直接返回、不设限。
func LimitRequestBody(c *gin.Context, maxBytes int64) {
	if c == nil || c.Request == nil || c.Request.Body == nil || maxBytes <= 0 {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
}

// IsRequestBodyTooLarge 判断 err 链中是否含 *http.MaxBytesError，即请求 body 是否超过了
// LimitRequestBody 设定的硬上限，便于上游据此返回 413 Request Entity Too Large。
func IsRequestBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// SingleValueHeader 读取并校验单值 header：去空白后忽略空值，值含逗号视为非法
// （ErrInvalidHeaderValue），出现多个非空值视为重复（ErrDuplicateHeader）。
// 恰好一个非空值时返回该值，全部为空时返回空串且无错误。
func SingleValueHeader(c *gin.Context, headerKey string) (string, error) {
	if c == nil || c.Request == nil {
		return "", nil
	}
	values := c.Request.Header.Values(headerKey)
	nonEmpty := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.Contains(value, ",") {
			return "", ErrInvalidHeaderValue
		}
		nonEmpty = append(nonEmpty, value)
	}
	if len(nonEmpty) > 1 {
		return "", ErrDuplicateHeader
	}
	if len(nonEmpty) == 1 {
		return nonEmpty[0], nil
	}
	return "", nil
}

// skipBodyParse 判断是否应跳过 body 解析，返回跳过原因：body 为 nil、GET/HEAD 方法返回 ErrNoBody，
// ContentLength 超过 MaxBodyBytes 返回 ErrBodyTooLarge，可解析时返回 nil。
func skipBodyParse(c *gin.Context) error {
	if c.Request == nil || c.Request.Body == nil {
		return ErrNoBody
	}
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		return ErrNoBody
	}
	if c.Request.ContentLength > int64(MaxBodyBytes) {
		return ErrBodyTooLarge
	}
	return nil
}

// parseRawBodySources 按 Content-Type 解析原始 body：application/json 以 UseNumber 解析为
// map（数字保留原始字面量、不丢精度），application/x-www-form-urlencoded 解析为 url.Values；
// 其他类型或解析失败返回 Available 为 false 并携带相应 Err。
func parseRawBodySources(contentType string, rawBody []byte) BodySources {
	switch strings.ToLower(contentType) {
	case "application/json":
		dec := gtkitjson.NewDecoder(bytes.NewReader(rawBody))
		dec.UseNumber()
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			return BodySources{Err: fmt.Errorf("%w: %w", ErrMalformedBody, err)}
		}
		if dec.More() {
			return BodySources{Err: fmt.Errorf("%w: unexpected trailing data", ErrMalformedBody)}
		}
		return BodySources{Available: true, JSON: m}
	case "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(rawBody))
		if err != nil {
			return BodySources{Err: fmt.Errorf("%w: %w", ErrMalformedBody, err)}
		}
		return BodySources{Available: true, Form: values}
	default:
		return BodySources{Err: ErrUnsupportedContentType}
	}
}

// readAndRestoreBody 读取至多 MaxBodyBytes+1 字节的 body（多读 1 字节用于判断是否超限），
// 并通过 multiReadCloser 把已读内容拼回 c.Request.Body，保证后续 ShouldBind 仍能完整读取。
// 对 ContentLength 已知与 chunked（未知长度）两种情况分别处理。
func readAndRestoreBody(c *gin.Context) ([]byte, error) {
	body := c.Request.Body
	limit := int64(MaxBodyBytes) + 1
	if c.Request.ContentLength >= 0 {
		raw, err := io.ReadAll(io.LimitReader(body, limit))
		c.Request.Body = newMultiReadCloser(bytes.NewReader(raw), body)
		if err != nil {
			return raw, fmt.Errorf("read body: %w", err)
		}
		return raw, nil
	}
	peeked, err := io.ReadAll(io.LimitReader(body, limit))
	c.Request.Body = newMultiReadCloser(io.MultiReader(bytes.NewReader(peeked), body), body)
	if err != nil {
		return peeked, fmt.Errorf("read chunked body: %w", err)
	}
	return peeked, nil
}

type multiReadCloser struct {
	reader io.Reader
	closer io.Closer
}

// newMultiReadCloser 用 reader 作为读取源、closer 作为关闭源，构造一个 io.ReadCloser，
// 使读取与关闭来源解耦，在不丢失原始 body Close 语义的前提下替换已被读取的 body。
func newMultiReadCloser(r io.Reader, c io.Closer) io.ReadCloser {
	return &multiReadCloser{reader: r, closer: c}
}

// Read 从 reader 读取，直接透传底层错误以保留 io.EOF 等哨兵错误。
//
//nolint:wrapcheck // Preserve io.Reader sentinel errors such as io.EOF.
func (m *multiReadCloser) Read(p []byte) (int, error) {
	return m.reader.Read(p)
}

// Close 调用 closer 关闭原始 body，直接透传其错误以保留底层关闭语义。
//
//nolint:wrapcheck // Preserve the wrapped body close semantics.
func (m *multiReadCloser) Close() error {
	return m.closer.Close()
}
