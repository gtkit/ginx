package ginx

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func BenchmarkParseBodyJSON(b *testing.B) {
	const body = `{"token":"body-token","scene_id":"abc","count":3}`
	b.ReportAllocs()
	for b.Loop() {
		c := newContext(http.MethodPost, "/x", "application/json", body)
		if got := ParseBody(c); !got.Available {
			b.Fatal("ParseBody not available")
		}
	}
}

func BenchmarkParseBodyForm(b *testing.B) {
	const body = "token=body-token&scene_id=abc&count=3"
	b.ReportAllocs()
	for b.Loop() {
		c := newContext(http.MethodPost, "/x", "application/x-www-form-urlencoded", body)
		if got := ParseBody(c); !got.Available {
			b.Fatal("ParseBody not available")
		}
	}
}

func BenchmarkParseBodyCached(b *testing.B) {
	c := newContext(http.MethodPost, "/x", "application/json", `{"k":"v"}`)
	if got := ParseBody(c); !got.Available {
		b.Fatal("ParseBody not available")
	}
	b.ReportAllocs()
	for b.Loop() {
		if got := ParseBody(c); !got.Available {
			b.Fatal("cache miss")
		}
	}
}

func BenchmarkQueryInt(b *testing.B) {
	c := newContext(http.MethodGet, "/x?page=42", "", "")
	b.ReportAllocs()
	for b.Loop() {
		if got := Query(c, "page", 1); got != 42 {
			b.Fatalf("Query = %d", got)
		}
	}
}

func BenchmarkRawBodyCached(b *testing.B) {
	c := newContext(http.MethodPost, "/x", "application/json", `{"k":"v"}`)
	if _, err := RawBody(c); err != nil {
		b.Fatalf("RawBody err = %v", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := RawBody(c); err != nil {
			b.Fatal("cache miss")
		}
	}
}

func BenchmarkBodyString(b *testing.B) {
	const body = `{"token":"body-token"}`
	b.ReportAllocs()
	for b.Loop() {
		c := newContext(http.MethodPost, "/x", "application/json", body)
		if got := BodyString(c, "token"); got != "body-token" {
			b.Fatalf("BodyString = %q", got)
		}
	}
}

func BenchmarkBodyInt(b *testing.B) {
	const body = `{"count":42}`
	b.ReportAllocs()
	for b.Loop() {
		c := newContext(http.MethodPost, "/x", "application/json", body)
		if got := Body(c, "count", 0); got != 42 {
			b.Fatalf("Body[int] = %d", got)
		}
	}
}

func BenchmarkBodyCached(b *testing.B) {
	c := newContext(http.MethodPost, "/x", "application/json", `{"k":"v"}`)
	Body(c, "k", "")
	b.ReportAllocs()
	for b.Loop() {
		if got := Body(c, "k", ""); got != "v" {
			b.Fatal("cache miss")
		}
	}
}

func BenchmarkHeaderInt(b *testing.B) {
	c := newContext(http.MethodGet, "/x", "", "")
	c.Request.Header.Set("X-Count", "42")
	b.ReportAllocs()
	for b.Loop() {
		if got := Header(c, "X-Count", int64(0)); got != 42 {
			b.Fatalf("Header[int64] = %d", got)
		}
	}
}

func BenchmarkHeaderString(b *testing.B) {
	c := newContext(http.MethodGet, "/x", "", "")
	c.Request.Header.Set("X-Token", "abc123")
	b.ReportAllocs()
	for b.Loop() {
		if got := Header(c, "X-Token", ""); got != "abc123" {
			b.Fatalf("Header[string] = %q", got)
		}
	}
}

func BenchmarkQuerySliceInt(b *testing.B) {
	c := newContext(http.MethodGet, "/x?id=1&id=2&id=3", "", "")
	b.ReportAllocs()
	for b.Loop() {
		if got := QuerySlice[int64](c, "id"); len(got) != 3 || got[0] != 1 {
			b.Fatalf("QuerySlice = %v", got)
		}
	}
}

func BenchmarkQuerySliceString(b *testing.B) {
	c := newContext(http.MethodGet, "/x?name=a&name=b&name=c", "", "")
	b.ReportAllocs()
	for b.Loop() {
		if got := QuerySlice[string](c, "name"); len(got) != 3 || got[0] != "a" {
			b.Fatalf("QuerySlice = %q", got)
		}
	}
}

func BenchmarkContextValue(b *testing.B) {
	c := &gin.Context{}
	c.Set("user_id", int64(42))
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := ContextValue[int64](c, "user_id"); !ok {
			b.Fatal("ContextValue not ok")
		}
	}
}
