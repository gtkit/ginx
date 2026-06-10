package ginx

import (
	"errors"
	"net/http"
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
