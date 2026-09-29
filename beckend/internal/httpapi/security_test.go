package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStrictJSON(t *testing.T) {
	for _, body := range []string{`{"name":"ok"} {"name":"again"}`, `{"name":"ok","admin":true}`, strings.Repeat("x", 17000)} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		var payload struct {
			Name string `json:"name"`
		}
		if decode(w, r, &payload) {
			t.Fatalf("accepted invalid input")
		}
	}
}
func TestRateLimitExpires(t *testing.T) {
	l := &loginLimiter{entries: map[string]attempts{}}
	for i := 0; i < 20; i++ {
		if !l.allow("one") {
			t.Fatal("blocked too early")
		}
	}
	if l.allow("one") {
		t.Fatal("limit not enforced")
	}
	if !l.allow("two") {
		t.Fatal("unrelated client blocked")
	}
	l.entries["one"] = attempts{count: 20, until: time.Now().Add(-time.Second)}
	if !l.allow("one") {
		t.Fatal("limit did not expire")
	}
}
