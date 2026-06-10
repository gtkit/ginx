package ginx

import (
	"net/http"
	"testing"
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
