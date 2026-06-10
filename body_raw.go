package ginx

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
)

// ctxKeyRawBodyCache 是原始 body 字节在 gin.Context 上的缓存键，使同一请求内只读取一次完整 body。
const ctxKeyRawBodyCache = "ginx.raw_body_cache"

// RawBody 读取并返回完整的原始请求 body：读取后回填 body（后续 BindBody / ShouldBind / ParseBody
// 仍可完整读取），并按请求缓存，同一请求内重复调用不再读流。适合 webhook 验签等先取原始字节、
// 再绑定结构体的场景。
//
// 本函数不设长度上限：请配合 LimitRequestBody 使用，超过硬上限时返回的错误可用
// IsRequestBodyTooLarge 判定。context、Request 或 body 为 nil 时返回 ErrNoBody；
// 读取失败时如实返回错误且不缓存。
func RawBody(c *gin.Context) ([]byte, error) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return nil, ErrNoBody
	}
	if v, ok := c.Get(ctxKeyRawBodyCache); ok {
		if cached, typeOK := v.([]byte); typeOK {
			return cached, nil
		}
	}
	body := c.Request.Body
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read raw body: %w", err)
	}
	c.Request.Body = newMultiReadCloser(bytes.NewReader(raw), body)
	c.Set(ctxKeyRawBodyCache, raw)
	return raw, nil
}

// BindBodyCached 基于 RawBody 缓存的原始字节将 body 绑定到 obj：同一请求内可重复调用，每次绑定
// 都读到完整 body，并继承 BindBody 的只信 body、不信 query 语义。首次调用会完整读取并缓存 body，
// 长度防御同样依赖 LimitRequestBody。context、Request 或 URL 为 nil 时返回 ErrInvalidBindContext。
func BindBodyCached(c *gin.Context, obj any) error {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ErrInvalidBindContext
	}
	raw, err := RawBody(c)
	if err != nil {
		return err
	}
	c.Request.Body = newMultiReadCloser(bytes.NewReader(raw), c.Request.Body)
	return BindBody(c, obj)
}

// RequireContentType 校验请求的 Content-Type（忽略参数与大小写）是否在 types 白名单内：命中返回
// nil；不命中返回可用 errors.Is 判定 ErrUnsupportedContentType 的错误并附实际类型，便于上游返回
// 415 Unsupported Media Type。types 为空时不做约束、返回 nil；context 或 Request 为 nil 视为
// 无 Content-Type。
func RequireContentType(c *gin.Context, types ...string) error {
	if len(types) == 0 {
		return nil
	}
	var got string
	if c != nil && c.Request != nil {
		got = c.ContentType()
	}
	if got != "" {
		for _, t := range types {
			if strings.EqualFold(got, t) {
				return nil
			}
		}
	}
	return fmt.Errorf("%w: %q", ErrUnsupportedContentType, got)
}
