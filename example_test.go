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
