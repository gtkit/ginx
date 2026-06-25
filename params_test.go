package ginx

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSingleValueQuery(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		want    string
		wantErr error
	}{
		{name: "missing", target: "/x"},
		{name: "single", target: "/x?id=abc"},
		{name: "single with spaces", target: "/x?id=%20abc%20"},
		{name: "blank ignored", target: "/x?id=&id=%20%20"},
		{name: "comma is legal", target: "/x?id=a,b"},
		{name: "duplicate", target: "/x?id=1&id=2", wantErr: ErrDuplicateQuery},
		{name: "duplicate after blank", target: "/x?id=&id=1&id=2", wantErr: ErrDuplicateQuery},
	}
	wants := map[string]string{"single": "abc", "single with spaces": "abc", "comma is legal": "a,b"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(http.MethodGet, tt.target, "", "")
			got, err := SingleValueQuery(c, "id")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if want := wants[tt.name]; got != want {
				t.Fatalf("got = %q, want %q", got, want)
			}
		})
	}

	if got, err := SingleValueQuery(nil, "id"); err != nil || got != "" {
		t.Fatalf("nil context got %q err %v, want empty nil", got, err)
	}
}

func TestQuery(t *testing.T) {
	c := newContext(http.MethodGet, "/x?page=3&big=9007199254740993&u=18446744073709551615&ok=true&ratio=0.5&name=%20alice%20&bad=abc&blank=%20%20", "", "")

	if got := Query(c, "page", 1); got != 3 {
		t.Fatalf("int = %d, want 3", got)
	}
	if got := Query(c, "big", int64(0)); got != 9007199254740993 {
		t.Fatalf("int64 = %d, want 9007199254740993", got)
	}
	if got := Query(c, "u", uint64(0)); got != 18446744073709551615 {
		t.Fatalf("uint64 = %d, want max", got)
	}
	if got := Query(c, "ok", false); got != true {
		t.Fatalf("bool = %v, want true", got)
	}
	if got := Query(c, "ratio", 0.0); got != 0.5 {
		t.Fatalf("float64 = %v, want 0.5", got)
	}
	if got := Query(c, "name", ""); got != "alice" {
		t.Fatalf("string = %q, want alice (trimmed)", got)
	}
	if got := Query(c, "bad", 7); got != 7 {
		t.Fatalf("invalid int = %d, want default 7", got)
	}
	if got := Query(c, "blank", 7); got != 7 {
		t.Fatalf("blank = %d, want default 7", got)
	}
	if got := Query(c, "missing", 7); got != 7 {
		t.Fatalf("missing = %d, want default 7", got)
	}
	if got := Query(nil, "page", 7); got != 7 {
		t.Fatalf("nil context = %d, want default 7", got)
	}
}

func TestQueryStrict(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr error
		want    int64
	}{
		{name: "ok", target: "/x?id=42", want: 42},
		{name: "trimmed", target: "/x?id=%2042%20", want: 42},
		{name: "missing", target: "/x", wantErr: ErrQueryMissing},
		{name: "blank", target: "/x?id=%20%20", wantErr: ErrQueryMissing},
		{name: "duplicate", target: "/x?id=1&id=2", wantErr: ErrDuplicateQuery},
		{name: "invalid", target: "/x?id=abc", wantErr: ErrInvalidQueryValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(http.MethodGet, tt.target, "", "")
			got, err := QueryStrict[int64](c, "id")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && got != tt.want {
				t.Fatalf("got = %d, want %d", got, tt.want)
			}
			if tt.wantErr != nil && got != 0 {
				t.Fatalf("got = %d on error, want zero", got)
			}
		})
	}

	// 非法值错误应附带原值，便于定位
	c := newContext(http.MethodGet, "/x?id=abc", "", "")
	if _, err := QueryStrict[int64](c, "id"); err == nil || !strings.Contains(err.Error(), "abc") {
		t.Fatalf("invalid err = %v, want contain %q", err, "abc")
	}

	// string 类型：非空即合法
	c = newContext(http.MethodGet, "/x?name=%20alice%20", "", "")
	if got, err := QueryStrict[string](c, "name"); err != nil || got != "alice" {
		t.Fatalf("string got %q err %v, want alice nil", got, err)
	}

	if _, err := QueryStrict[int64](nil, "id"); !errors.Is(err, ErrQueryMissing) {
		t.Fatalf("nil context err = %v, want ErrQueryMissing", err)
	}
}

func TestQuerySliceStrict(t *testing.T) {
	// 全部合法
	c := newContext(http.MethodGet, "/x?id=1&id=2&id=3", "", "")
	got, err := QuerySliceStrict[int64](c, "id")
	if err != nil || len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("got %v err %v, want [1 2 3] nil", got, err)
	}

	// 含非法值 → 报错并附带该值，不静默跳过
	c = newContext(http.MethodGet, "/x?id=1&id=abc&id=3", "", "")
	got, err = QuerySliceStrict[int64](c, "id")
	if !errors.Is(err, ErrInvalidQueryValue) || got != nil {
		t.Fatalf("got %v err %v, want nil ErrInvalidQueryValue", got, err)
	}
	if !strings.Contains(err.Error(), "abc") {
		t.Fatalf("err = %v, want contain %q", err, "abc")
	}

	// 含空值 → 严格模式视为非法
	c = newContext(http.MethodGet, "/x?id=1&id=%20&id=3", "", "")
	if _, err := QuerySliceStrict[int64](c, "id"); !errors.Is(err, ErrInvalidQueryValue) {
		t.Fatalf("blank err = %v, want ErrInvalidQueryValue", err)
	}

	// 缺失 key → (nil, nil)
	c = newContext(http.MethodGet, "/x", "", "")
	if got, err := QuerySliceStrict[int64](c, "id"); err != nil || got != nil {
		t.Fatalf("missing got %v err %v, want nil nil", got, err)
	}

	// nil context → (nil, nil)
	if got, err := QuerySliceStrict[int64](nil, "id"); err != nil || got != nil {
		t.Fatalf("nil context got %v err %v, want nil nil", got, err)
	}
}

func TestParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newContext(http.MethodGet, "/x", "", "")
	c.Params = gin.Params{
		{Key: "id", Value: "42"},
		{Key: "name", Value: "alice"},
		{Key: "bad", Value: "x"},
	}

	if got := Param(c, "id", int64(0)); got != 42 {
		t.Fatalf("int64 = %d, want 42", got)
	}
	if got := Param(c, "name", ""); got != "alice" {
		t.Fatalf("string = %q, want alice", got)
	}
	if got := Param(c, "bad", 7); got != 7 {
		t.Fatalf("invalid = %d, want default 7", got)
	}
	if got := Param(c, "missing", 7); got != 7 {
		t.Fatalf("missing = %d, want default 7", got)
	}
	if got := Param(nil, "id", 7); got != 7 {
		t.Fatalf("nil context = %d, want default 7", got)
	}
}

func TestBody(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		ct      string
		body    string
		field   string
		wantInt int64
	}{
		{name: "json int", method: http.MethodPost, ct: "application/json", body: `{"id":42}`, field: "id", wantInt: 42},
		{name: "form int", method: http.MethodPost, ct: "application/x-www-form-urlencoded", body: "id=99", field: "id", wantInt: 99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(tt.method, "/x", tt.ct, tt.body)
			if got := Body(c, tt.field, int64(0)); got != tt.wantInt {
				t.Fatalf("Body[int64] = %d, want %d", got, tt.wantInt)
			}
		})
	}

	// 字符串字段
	c := newContext(http.MethodPost, "/x", "application/json", `{"name":" alice "}`)
	if got := Body(c, "name", ""); got != "alice" {
		t.Fatalf("Body[string] = %q, want alice", got)
	}

	// uint64 字段
	c = newContext(http.MethodPost, "/x", "application/json", `{"u":18446744073709551615}`)
	if got := Body(c, "u", uint64(0)); got != 18446744073709551615 {
		t.Fatalf("Body[uint64] = %d, want max", got)
	}

	// bool 字段
	c = newContext(http.MethodPost, "/x", "application/json", `{"ok":true}`)
	if got := Body(c, "ok", false); got != true {
		t.Fatalf("Body[bool] = %v, want true", got)
	}

	// float64 字段
	c = newContext(http.MethodPost, "/x", "application/json", `{"ratio":3.14}`)
	if got := Body(c, "ratio", 0.0); got != 3.14 {
		t.Fatalf("Body[float64] = %v, want 3.14", got)
	}

	// 大整数保持精度
	c = newContext(http.MethodPost, "/x", "application/json", `{"big":9007199254740993}`)
	if got := Body(c, "big", int64(0)); got != 9007199254740993 {
		t.Fatalf("Body[int64] large = %d, want 9007199254740993", got)
	}

	// form 字符串
	c = newContext(http.MethodPost, "/x", "application/x-www-form-urlencoded", "name=bob")
	if got := Body(c, "name", ""); got != "bob" {
		t.Fatalf("Body[string] form = %q, want bob", got)
	}

	// 缺失字段 → 默认值
	c = newContext(http.MethodPost, "/x", "application/json", `{"a":1}`)
	if got := Body(c, "missing", "def"); got != "def" {
		t.Fatalf("missing field = %q, want 'def'", got)
	}

	// 不可解析的 body → 默认值
	c = newContext(http.MethodPost, "/x", "text/plain", "hello")
	if got := Body(c, "name", "def"); got != "def" {
		t.Fatalf("unsupported ct = %q, want 'def'", got)
	}

	// GET 请求 → 默认值
	c = newContext(http.MethodGet, "/x", "application/json", `{"name":"alice"}`)
	if got := Body(c, "name", "def"); got != "def" {
		t.Fatalf("GET = %q, want 'def'", got)
	}

	// nil context → 默认值
	if got := Body(nil, "name", "def"); got != "def" {
		t.Fatalf("nil context = %q, want 'def'", got)
	}
}

func TestQuerySlice(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		key     string
		wantLen int
		want0   int64
		want1   int64
	}{
		{name: "missing", target: "/x", wantLen: 0},
		{name: "single", target: "/x?id=42", key: "id", wantLen: 1, want0: 42},
		{name: "multiple", target: "/x?id=1&id=2&id=3", key: "id", wantLen: 3, want0: 1, want1: 2},
		{name: "mixed valid/invalid", target: "/x?id=1&id=abc&id=3", key: "id", wantLen: 2, want0: 1, want1: 3},
		{name: "blank skipped", target: "/x?id=1&id=%20&id=3", key: "id", wantLen: 2, want0: 1, want1: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(http.MethodGet, tt.target, "", "")
			got := QuerySlice[int64](c, tt.key)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d; got=%v", len(got), tt.wantLen, got)
			}
			if tt.wantLen >= 1 && got[0] != tt.want0 {
				t.Fatalf("got[0] = %d, want %d", got[0], tt.want0)
			}
			if tt.wantLen >= 2 && got[1] != tt.want1 {
				t.Fatalf("got[1] = %d, want %d", got[1], tt.want1)
			}
		})
	}

	// string slice
	c := newContext(http.MethodGet, "/x?name=a&name=b&name=c", "", "")
	ss := QuerySlice[string](c, "name")
	if len(ss) != 3 || ss[0] != "a" || ss[2] != "c" {
		t.Fatalf("string slice = %v, want [a b c]", ss)
	}

	// bool slice
	c = newContext(http.MethodGet, "/x?flag=true&flag=false&flag=t", "", "")
	bs := QuerySlice[bool](c, "flag")
	if len(bs) != 3 || bs[0] != true || bs[1] != false {
		t.Fatalf("bool slice = %v, want [true false true]", bs)
	}

	// uint64 slice
	c = newContext(http.MethodGet, "/x?u=1&u=2", "", "")
	us := QuerySlice[uint64](c, "u")
	if len(us) != 2 || us[0] != 1 || us[1] != 2 {
		t.Fatalf("uint64 slice = %v, want [1 2]", us)
	}

	// float64 slice
	c = newContext(http.MethodGet, "/x?r=1.5&r=2.5", "", "")
	fs := QuerySlice[float64](c, "r")
	if len(fs) != 2 || fs[0] != 1.5 || fs[1] != 2.5 {
		t.Fatalf("float64 slice = %v, want [1.5 2.5]", fs)
	}

	// nil context → nil
	if got := QuerySlice[int64](nil, "id"); got != nil {
		t.Fatalf("nil context = %v, want nil", got)
	}
}
