package ginx

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type trackingReadCloser struct {
	reader io.Reader
	closed bool
	err    error
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	return r.reader.Read(p)
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

type fixedContentLengthBody struct {
	payload []byte
	offset  int
}

func (b *fixedContentLengthBody) Read(p []byte) (int, error) {
	if b.offset >= len(b.payload) {
		return 0, io.EOF
	}
	n := copy(p, b.payload[b.offset:])
	b.offset += n
	return n, nil
}

func (b *fixedContentLengthBody) Close() error {
	return nil
}

func newContext(method, target, contentType, body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	c.Request = req
	return c
}

func TestSingleValueHeader(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    string
		wantErr error
	}{
		{name: "missing"},
		{name: "single", values: []string{" abc "}, want: "abc"},
		{name: "blank ignored", values: []string{"", "  "}},
		{name: "duplicate", values: []string{"a", "b"}, wantErr: ErrDuplicateHeader},
		{name: "comma", values: []string{"a,b"}, wantErr: ErrInvalidHeaderValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(http.MethodGet, "/x", "", "")
			for _, v := range tt.values {
				c.Request.Header.Add("X-Token", v)
			}

			got, err := SingleValueHeader(c, "X-Token")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got = %q, want %q", got, tt.want)
			}
		})
	}

	if got, err := SingleValueHeader(nil, "X-Token"); err != nil || got != "" {
		t.Fatalf("nil context got %q err %v, want empty nil", got, err)
	}
}

func TestParseBody(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		ct       string
		body     string
		wantOK   bool
		wantJSON bool
		wantForm bool
		wantErr  error
	}{
		{name: "json", method: http.MethodPost, ct: "application/json", body: `{"k":"v"}`, wantOK: true, wantJSON: true},
		{name: "json mixed case with charset", method: http.MethodPost, ct: "Application/JSON; charset=utf-8", body: `{"k":"v"}`, wantOK: true, wantJSON: true},
		{name: "form", method: http.MethodPost, ct: "application/x-www-form-urlencoded", body: "k=v", wantOK: true, wantForm: true},
		{name: "unsupported content type", method: http.MethodPost, ct: "text/plain", body: "k=v", wantErr: ErrUnsupportedContentType},
		{name: "get skipped", method: http.MethodGet, ct: "application/json", body: `{"k":"v"}`, wantErr: ErrNoBody},
		{name: "invalid json", method: http.MethodPost, ct: "application/json", body: `{`, wantErr: ErrMalformedBody},
		{name: "json trailing data", method: http.MethodPost, ct: "application/json", body: `{"k":"v"} x`, wantErr: ErrMalformedBody},
		{name: "invalid form encoding", method: http.MethodPost, ct: "application/x-www-form-urlencoded", body: "k=%zz", wantErr: ErrMalformedBody},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(tt.method, "/x", tt.ct, tt.body)
			got := ParseBody(c)
			if got.Available != tt.wantOK {
				t.Fatalf("Available = %v, want %v", got.Available, tt.wantOK)
			}
			if !errors.Is(got.Err, tt.wantErr) {
				t.Fatalf("Err = %v, want %v", got.Err, tt.wantErr)
			}
			if tt.wantOK && got.Err != nil {
				t.Fatalf("Err = %v, want nil on success", got.Err)
			}
			if tt.wantJSON && got.JSON == nil {
				t.Fatal("want JSON map, got nil")
			}
			if tt.wantForm && got.Form == nil {
				t.Fatal("want Form values, got nil")
			}
		})
	}

	if got := ParseBody(nil); got.Available || !errors.Is(got.Err, ErrNoBody) {
		t.Fatalf("nil context got Available=%v Err=%v, want false ErrNoBody", got.Available, got.Err)
	}
}

func TestParseBodyCachesFailureReason(t *testing.T) {
	c := newContext(http.MethodPost, "/x", "text/plain", "k=v")

	first := ParseBody(c)
	if !errors.Is(first.Err, ErrUnsupportedContentType) {
		t.Fatalf("first Err = %v, want ErrUnsupportedContentType", first.Err)
	}
	second := ParseBody(c)
	if !errors.Is(second.Err, ErrUnsupportedContentType) {
		t.Fatalf("second Err = %v, want ErrUnsupportedContentType", second.Err)
	}
}

func TestParseBodyReadErrorExposed(t *testing.T) {
	c := newContext(http.MethodPost, "/x", "application/json", "")
	c.Request.Body = &trackingReadCloser{err: errors.New("boom")}
	c.Request.ContentLength = 10

	got := ParseBody(c)
	if got.Available {
		t.Fatal("Available = true, want false")
	}
	if got.Err == nil || !strings.Contains(got.Err.Error(), "boom") {
		t.Fatalf("Err = %v, want wrapped read error", got.Err)
	}
}

func TestParseBodyChunkedOversize(t *testing.T) {
	body := strings.Repeat("a", MaxBodyBytes+10)
	c := newContext(http.MethodPost, "/x", "application/json", "")
	c.Request.Body = &trackingReadCloser{reader: strings.NewReader(body)}
	c.Request.ContentLength = -1

	got := ParseBody(c)
	if got.Available || !errors.Is(got.Err, ErrBodyTooLarge) {
		t.Fatalf("got Available=%v Err=%v, want false ErrBodyTooLarge", got.Available, got.Err)
	}
}

func TestParseBodyPreservesBody(t *testing.T) {
	const body = `{"token":"body-token"}`
	c := newContext(http.MethodPost, "/x", "application/json", body)

	if got := ParseBody(c); !got.Available {
		t.Fatal("ParseBody not available")
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		t.Fatalf("read restored body: %v", err)
	}
	if string(raw) != body {
		t.Fatalf("restored body = %q, want %q", string(raw), body)
	}
}

func TestParseBodyKnownOversizeDoesNotRead(t *testing.T) {
	body := strings.Repeat("a", MaxBodyBytes+1)
	rc := &trackingReadCloser{reader: strings.NewReader(body)}
	c := newContext(http.MethodPost, "/x", "application/json", "")
	c.Request.Body = rc
	c.Request.ContentLength = int64(len(body))

	if got := ParseBody(c); got.Available || !errors.Is(got.Err, ErrBodyTooLarge) {
		t.Fatalf("got Available=%v Err=%v, want false ErrBodyTooLarge", got.Available, got.Err)
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(raw) != body {
		t.Fatal("body changed")
	}
}

func TestBodyString(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		ct      string
		body    string
		field   string
		want    string
		wantRaw string
	}{
		{
			name:    "string field",
			method:  http.MethodPost,
			ct:      "application/json",
			body:    `{"token":" body-token "}`,
			field:   "token",
			want:    "body-token",
			wantRaw: `{"token":" body-token "}`,
		},
		{
			name:    "form field",
			method:  http.MethodPost,
			ct:      "application/x-www-form-urlencoded",
			body:    `token=+form-token+`,
			field:   "token",
			want:    "form-token",
			wantRaw: `token=+form-token+`,
		},
		{
			name:    "missing field",
			method:  http.MethodPost,
			ct:      "application/json",
			body:    `{"scene_id":"abc"}`,
			field:   "token",
			wantRaw: `{"scene_id":"abc"}`,
		},
		{
			name:    "numeric json field",
			method:  http.MethodPost,
			ct:      "application/json",
			body:    `{"token":12345}`,
			field:   "token",
			want:    "12345",
			wantRaw: `{"token":12345}`,
		},
		{
			name:    "boolean json field",
			method:  http.MethodPost,
			ct:      "application/json",
			body:    `{"token":true}`,
			field:   "token",
			want:    "true",
			wantRaw: `{"token":true}`,
		},
		{
			name:    "big integer keeps precision",
			method:  http.MethodPost,
			ct:      "application/json",
			body:    `{"id":9007199254740993}`,
			field:   "id",
			want:    "9007199254740993",
			wantRaw: `{"id":9007199254740993}`,
		},
		{
			name:    "decimal literal preserved",
			method:  http.MethodPost,
			ct:      "application/json",
			body:    `{"price":1.50}`,
			field:   "price",
			want:    "1.50",
			wantRaw: `{"price":1.50}`,
		},
		{
			name:    "object json field ignored",
			method:  http.MethodPost,
			ct:      "application/json",
			body:    `{"token":{"value":"abc"}}`,
			field:   "token",
			wantRaw: `{"token":{"value":"abc"}}`,
		},
		{
			name:    "invalid json",
			method:  http.MethodPost,
			ct:      "application/json",
			body:    `{"token":`,
			field:   "token",
			wantRaw: `{"token":`,
		},
		{
			name:    "get skips body",
			method:  http.MethodGet,
			ct:      "application/json",
			body:    `{"token":"body-token"}`,
			field:   "token",
			wantRaw: `{"token":"body-token"}`,
		},
		{
			name:    "unsupported content type ignored",
			method:  http.MethodPost,
			ct:      "text/plain",
			body:    `token=body-token`,
			field:   "token",
			wantRaw: `token=body-token`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(tt.method, "/x", tt.ct, tt.body)

			if got := BodyString(c, tt.field); got != tt.want {
				t.Fatalf("BodyString = %q, want %q", got, tt.want)
			}
			raw, err := io.ReadAll(c.Request.Body)
			if err != nil {
				t.Fatalf("read restored body: %v", err)
			}
			if string(raw) != tt.wantRaw {
				t.Fatalf("restored body = %q, want %q", string(raw), tt.wantRaw)
			}
		})
	}
}

func TestBindBody(t *testing.T) {
	type request struct {
		Token string `json:"token" binding:"required" form:"token"`
	}

	tests := []struct {
		name       string
		target     string
		ct         string
		body       string
		want       string
		wantErr    bool
		wantRawURL string
	}{
		{
			name:       "json body",
			target:     "/x?token=query-token",
			ct:         "application/json",
			body:       `{"token":"body-token"}`,
			want:       "body-token",
			wantRawURL: "token=query-token",
		},
		{
			name:       "form body",
			target:     "/x?token=query-token",
			ct:         "application/x-www-form-urlencoded",
			body:       "token=form-token",
			want:       "form-token",
			wantRawURL: "token=query-token",
		},
		{
			name:       "query ignored",
			target:     "/x?token=query-token",
			ct:         "application/x-www-form-urlencoded",
			wantErr:    true,
			wantRawURL: "token=query-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(http.MethodPost, tt.target, tt.ct, tt.body)
			var req request

			err := BindBody(c, &req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("BindBody err = %v, wantErr %v", err, tt.wantErr)
			}
			if req.Token != tt.want {
				t.Fatalf("token = %q, want %q", req.Token, tt.want)
			}
			if got := c.Request.URL.RawQuery; got != tt.wantRawURL {
				t.Fatalf("RawQuery = %q, want %q", got, tt.wantRawURL)
			}
		})
	}
}

func TestBindBodyInvalidContextReturnsError(t *testing.T) {
	type request struct {
		Token string `json:"token" binding:"required" form:"token"`
	}

	tests := []struct {
		name string
		ctx  *gin.Context
	}{
		{name: "nil context"},
		{name: "nil request", ctx: &gin.Context{}},
		{name: "nil request url", ctx: &gin.Context{Request: &http.Request{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req request

			err := BindBody(tt.ctx, &req)
			if !errors.Is(err, ErrInvalidBindContext) {
				t.Fatalf("BindBody err = %v, want %v", err, ErrInvalidBindContext)
			}
			if req.Token != "" {
				t.Fatalf("token = %q, want empty", req.Token)
			}
		})
	}
}

func TestReadAndRestoreBodyLimitsKnownLengthRead(t *testing.T) {
	body := strings.Repeat("a", MaxBodyBytes*4)
	c := newContext(http.MethodPost, "/x", "application/json", "")
	c.Request.Body = &fixedContentLengthBody{payload: []byte(body)}
	c.Request.ContentLength = int64(MaxBodyBytes)

	raw, err := readAndRestoreBody(c)
	if err != nil {
		t.Fatalf("readAndRestoreBody err = %v", err)
	}
	if len(raw) != MaxBodyBytes+1 {
		t.Fatalf("len(raw) = %d, want %d", len(raw), MaxBodyBytes+1)
	}
}

func TestReadAndRestoreBodyKnownLengthReadError(t *testing.T) {
	c := newContext(http.MethodPost, "/x", "application/json", "")
	c.Request.Body = &trackingReadCloser{err: errors.New("boom")}
	c.Request.ContentLength = 10

	if _, err := readAndRestoreBody(c); err == nil {
		t.Fatal("want read error, got nil")
	}
}

func TestReadAndRestoreBodyChunkedRestoresFullBody(t *testing.T) {
	// 超过 peek 上限，验证剩余部分仍可从原 body 续读
	body := strings.Repeat("a", MaxBodyBytes+100)
	c := newContext(http.MethodPost, "/x", "application/json", "")
	c.Request.Body = &trackingReadCloser{reader: strings.NewReader(body)}
	c.Request.ContentLength = -1

	peeked, err := readAndRestoreBody(c)
	if err != nil {
		t.Fatalf("readAndRestoreBody err = %v", err)
	}
	if len(peeked) != MaxBodyBytes+1 {
		t.Fatalf("len(peeked) = %d, want %d", len(peeked), MaxBodyBytes+1)
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		t.Fatalf("read restored body: %v", err)
	}
	if string(raw) != body {
		t.Fatal("restored chunked body mismatch")
	}
}

func TestReadAndRestoreBodyChunkedReadError(t *testing.T) {
	c := newContext(http.MethodPost, "/x", "application/json", "")
	c.Request.Body = &trackingReadCloser{err: errors.New("boom")}
	c.Request.ContentLength = -1

	if _, err := readAndRestoreBody(c); err == nil {
		t.Fatal("want read error, got nil")
	}
}

func TestMultiReadCloserCloseClosesOriginal(t *testing.T) {
	orig := &trackingReadCloser{reader: strings.NewReader("abc")}
	rc := newMultiReadCloser(strings.NewReader("abc"), orig)
	if err := rc.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	if !orig.closed {
		t.Fatal("original closer was not closed")
	}
}

func TestLimitRequestBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		maxBytes int64
		wantErr  bool
		wantRead string
	}{
		{name: "within limit", body: "hello", maxBytes: 10, wantRead: "hello"},
		{name: "exactly at limit", body: "hello", maxBytes: 5, wantRead: "hello"},
		{name: "exceeds limit", body: "hello world", maxBytes: 5, wantErr: true},
		{name: "zero means no limit", body: "unbounded body", maxBytes: 0, wantRead: "unbounded body"},
		{name: "negative means no limit", body: "abc", maxBytes: -1, wantRead: "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(http.MethodPost, "/", "text/plain", tt.body)
			LimitRequestBody(c, tt.maxBytes)

			got, err := io.ReadAll(c.Request.Body)
			if tt.wantErr {
				if err == nil {
					t.Fatal("want read error, got nil")
				}
				if !IsRequestBodyTooLarge(err) {
					t.Fatalf("IsRequestBodyTooLarge = false, want true (err=%v)", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected read error: %v", err)
			}
			if string(got) != tt.wantRead {
				t.Fatalf("read = %q, want %q", got, tt.wantRead)
			}
		})
	}
}

func TestLimitRequestBodyNilSafe(t *testing.T) {
	// nil context 不应 panic
	LimitRequestBody(nil, 10)

	// context 无 Request 不应 panic
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	LimitRequestBody(c, 10)
}

func TestLimitRequestBodyWithBindBody(t *testing.T) {
	large := `{"token":"` + strings.Repeat("a", 100) + `"}`
	c := newContext(http.MethodPost, "/", "application/json", large)
	LimitRequestBody(c, 10)

	var dst struct {
		Token string `json:"token"`
	}
	err := BindBody(c, &dst)
	if err == nil {
		t.Fatal("want bind error from oversize body, got nil")
	}
	// BindBody 用 fmt.Errorf("bind body: %w", err) 包装，验证 errors.As 仍能透过包装识别
	if !IsRequestBodyTooLarge(err) {
		t.Fatalf("IsRequestBodyTooLarge = false on wrapped error, want true (err=%v)", err)
	}
}

func TestIsRequestBodyTooLarge(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("boom"), want: false},
		{name: "max bytes error", err: &http.MaxBytesError{Limit: 8}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRequestBodyTooLarge(tt.err); got != tt.want {
				t.Fatalf("IsRequestBodyTooLarge(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
