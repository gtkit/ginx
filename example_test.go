package ginx_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gtkit/ginx"
)

// newPostContext 构造一个带 body 的 POST 请求测试上下文，仅用于 Example。
func newPostContext(contentType, body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/demo", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	c.Request = req
	return c
}

func ExampleParseBody() {
	c := newPostContext("application/json", `{"name":"alice"}`)

	src := ginx.ParseBody(c)
	fmt.Println(src.Available, src.JSON["name"])
	// Output: true alice
}

func ExampleParseBody_failureReason() {
	c := newPostContext("text/plain", "hello")

	src := ginx.ParseBody(c)
	fmt.Println(src.Available, errors.Is(src.Err, ginx.ErrUnsupportedContentType))
	// Output: false true
}

func ExampleBodyString() {
	c := newPostContext("application/x-www-form-urlencoded", "token=abc123")

	fmt.Println(ginx.BodyString(c, "token"))
	// Output: abc123
}

func ExampleBindBody() {
	c := newPostContext("application/json", `{"token":"body-token"}`)
	c.Request.URL.RawQuery = "token=query-token" // query 不会被并入绑定结果

	var req struct {
		Token string `json:"token" form:"token"`
	}
	if err := ginx.BindBody(c, &req); err != nil {
		fmt.Println("bind:", err)
		return
	}
	fmt.Println(req.Token)
	// Output: body-token
}

func ExampleRawBody() {
	// webhook 验签场景：先取原始字节算签名，再绑定结构体，两者不互斥
	c := newPostContext("application/json", `{"event":"pay.success"}`)

	raw, err := ginx.RawBody(c)
	if err != nil {
		fmt.Println("read:", err)
		return
	}
	// verifySignature(raw, c.GetHeader("X-Signature")) ...

	var notify struct {
		Event string `json:"event"`
	}
	if err := ginx.BindBody(c, &notify); err != nil {
		fmt.Println("bind:", err)
		return
	}
	fmt.Println(len(raw) > 0, notify.Event)
	// Output: true pay.success
}

func ExampleBindBodyCached() {
	c := newPostContext("application/json", `{"token":"t1","scene":"s1"}`)

	var a struct {
		Token string `json:"token"`
	}
	var b struct {
		Scene string `json:"scene"`
	}
	_ = ginx.BindBodyCached(c, &a) // 同一请求可重复绑定
	_ = ginx.BindBodyCached(c, &b)
	fmt.Println(a.Token, b.Scene)
	// Output: t1 s1
}

func ExampleSingleValueQuery() {
	c := newPostContext("application/json", "{}")
	c.Request.URL.RawQuery = "id=1&id=2" // HTTP 参数污染

	_, err := ginx.SingleValueQuery(c, "id")
	fmt.Println(errors.Is(err, ginx.ErrDuplicateQuery))
	// Output: true
}

func ExampleQuery() {
	c := newPostContext("application/json", "{}")
	c.Request.URL.RawQuery = "page=3&size=abc"

	fmt.Println(ginx.Query(c, "page", 1), ginx.Query(c, "size", 20), ginx.Query(c, "dry_run", false))
	// Output: 3 20 false
}

func ExampleParam() {
	c := newPostContext("application/json", "{}")
	c.Params = gin.Params{{Key: "id", Value: "42"}} // 路由 /user/:id

	fmt.Println(ginx.Param(c, "id", int64(0)))
	// Output: 42
}

func ExampleRequireContentType() {
	c := newPostContext("text/plain", "hello")

	err := ginx.RequireContentType(c, "application/json", "application/x-www-form-urlencoded")
	fmt.Println(errors.Is(err, ginx.ErrUnsupportedContentType))
	// Output: true
}

func ExampleSingleValueHeader() {
	c := newPostContext("application/json", "{}")
	c.Request.Header.Add("X-Token", "a")
	c.Request.Header.Add("X-Token", "b")

	_, err := ginx.SingleValueHeader(c, "X-Token")
	fmt.Println(errors.Is(err, ginx.ErrDuplicateHeader))
	// Output: true
}

func ExampleLimitRequestBody() {
	c := newPostContext("application/json", `{"data":"`+strings.Repeat("a", 64)+`"}`)
	ginx.LimitRequestBody(c, 16) // 16 字节硬上限

	var req struct {
		Data string `json:"data"`
	}
	err := ginx.BindBody(c, &req)
	fmt.Println(ginx.IsRequestBodyTooLarge(err))
	// Output: true
}
