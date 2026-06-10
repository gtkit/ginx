package ginx

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRawBody(t *testing.T) {
	const body = `{"token":"body-token"}`
	c := newContext(http.MethodPost, "/x", "application/json", body)

	raw, err := RawBody(c)
	if err != nil {
		t.Fatalf("RawBody err = %v", err)
	}
	if string(raw) != body {
		t.Fatalf("raw = %q, want %q", raw, body)
	}

	// 回填后绑定仍可读到完整 body
	var dst struct {
		Token string `json:"token"`
	}
	if err := BindBody(c, &dst); err != nil {
		t.Fatalf("BindBody after RawBody err = %v", err)
	}
	if dst.Token != "body-token" {
		t.Fatalf("token = %q, want body-token", dst.Token)
	}
}

func TestRawBodySecondCallUsesCache(t *testing.T) {
	const body = "hello"
	c := newContext(http.MethodPost, "/x", "text/plain", body)

	first, err := RawBody(c)
	if err != nil {
		t.Fatalf("first RawBody err = %v", err)
	}
	// 换成必然报错的 reader，证明第二次不再读流
	c.Request.Body = &trackingReadCloser{err: errors.New("must not read")}
	second, err := RawBody(c)
	if err != nil {
		t.Fatalf("second RawBody err = %v", err)
	}
	if string(first) != body || string(second) != body {
		t.Fatalf("first = %q second = %q, want %q", first, second, body)
	}
}

func TestRawBodyNilSafe(t *testing.T) {
	if _, err := RawBody(nil); !errors.Is(err, ErrNoBody) {
		t.Fatalf("nil context err = %v, want ErrNoBody", err)
	}
	gin.SetMode(gin.TestMode)
	c := newContext(http.MethodGet, "/x", "", "")
	c.Request.Body = nil
	if _, err := RawBody(c); !errors.Is(err, ErrNoBody) {
		t.Fatalf("nil body err = %v, want ErrNoBody", err)
	}
}

func TestRawBodyTooLarge(t *testing.T) {
	c := newContext(http.MethodPost, "/x", "text/plain", strings.Repeat("a", 100))
	LimitRequestBody(c, 10)

	_, err := RawBody(c)
	if err == nil || !IsRequestBodyTooLarge(err) {
		t.Fatalf("err = %v, want MaxBytesError wrap", err)
	}
}

func TestRawBodyReadErrorNotCached(t *testing.T) {
	c := newContext(http.MethodPost, "/x", "text/plain", "")
	c.Request.Body = &trackingReadCloser{err: errors.New("boom")}

	if _, err := RawBody(c); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want wrapped boom", err)
	}
	// 失败不缓存：恢复正常 body 后可成功读取
	c.Request.Body = io.NopCloser(strings.NewReader("ok"))
	raw, err := RawBody(c)
	if err != nil || string(raw) != "ok" {
		t.Fatalf("retry got %q err %v, want ok nil", raw, err)
	}
}

func TestBindBodyCachedRepeatedBind(t *testing.T) {
	const body = `{"token":"body-token","scene":"abc"}`
	c := newContext(http.MethodPost, "/x?token=query-token", "application/json", body)

	var first struct {
		Token string `json:"token"`
	}
	if err := BindBodyCached(c, &first); err != nil {
		t.Fatalf("first bind err = %v", err)
	}
	var second struct {
		Token string `json:"token"`
		Scene string `json:"scene"`
	}
	if err := BindBodyCached(c, &second); err != nil {
		t.Fatalf("second bind err = %v", err)
	}
	if first.Token != "body-token" || second.Token != "body-token" || second.Scene != "abc" {
		t.Fatalf("first = %+v second = %+v", first, second)
	}
}

func TestBindBodyCachedFormIgnoresQuery(t *testing.T) {
	c := newContext(http.MethodPost, "/x?token=query-token", "application/x-www-form-urlencoded", "scene=abc")

	var dst struct {
		Token string `form:"token"`
		Scene string `form:"scene"`
	}
	if err := BindBodyCached(c, &dst); err != nil {
		t.Fatalf("bind err = %v", err)
	}
	if dst.Token != "" || dst.Scene != "abc" {
		t.Fatalf("dst = %+v, want query ignored", dst)
	}
}

func TestBindBodyCachedInvalidContext(t *testing.T) {
	var dst struct{}
	if err := BindBodyCached(nil, &dst); !errors.Is(err, ErrInvalidBindContext) {
		t.Fatalf("err = %v, want ErrInvalidBindContext", err)
	}
}

func TestBindBodyCachedNoBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newContext(http.MethodPost, "/x", "application/json", "")
	c.Request.Body = nil

	var dst struct{}
	if err := BindBodyCached(c, &dst); !errors.Is(err, ErrNoBody) {
		t.Fatalf("err = %v, want ErrNoBody", err)
	}
}

func TestRequireContentType(t *testing.T) {
	tests := []struct {
		name    string
		ct      string
		types   []string
		wantErr bool
	}{
		{name: "exact match", ct: "application/json", types: []string{"application/json"}},
		{name: "charset stripped", ct: "application/json; charset=utf-8", types: []string{"application/json"}},
		{name: "case insensitive", ct: "Application/JSON", types: []string{"application/json"}},
		{name: "second in whitelist", ct: "text/xml", types: []string{"application/json", "text/xml"}},
		{name: "mismatch", ct: "text/plain", types: []string{"application/json"}, wantErr: true},
		{name: "empty content type", ct: "", types: []string{"application/json"}, wantErr: true},
		{name: "no restriction", ct: "text/plain", types: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(http.MethodPost, "/x", tt.ct, "k=v")
			err := RequireContentType(c, tt.types...)
			if tt.wantErr {
				if !errors.Is(err, ErrUnsupportedContentType) {
					t.Fatalf("err = %v, want ErrUnsupportedContentType", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}

	if err := RequireContentType(nil, "application/json"); !errors.Is(err, ErrUnsupportedContentType) {
		t.Fatalf("nil context err = %v, want ErrUnsupportedContentType", err)
	}
}
