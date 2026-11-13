package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOriginAndContentType(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	handler := sameOrigin("https://manager.test", next)
	for _, tc := range []struct {
		origin, media, site string
		want                int
	}{
		{"https://manager.test", "application/json", "same-origin", 204},
		{"http://manager.test", "application/json", "same-site", 403},
		{"https://evil.test", "application/json", "cross-site", 403},
		{"null", "application/json", "", 403},
		{"", "text/plain", "", 415},
		{"", "application/x-www-form-urlencoded", "", 415},
		{"", "application/json", "", 204},
	} {
		r := httptest.NewRequest("POST", "https://manager.test/api/v1/auth/login", strings.NewReader(`{}`))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.media)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%+v: %d", tc, w.Code)
		}
	}
}
func TestLoginRateLimit(t *testing.T) {
	handler := loginThrottle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for i := 0; i < 11; i++ {
		r := httptest.NewRequest("POST", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		want := 204
		if i == 10 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d got %d", i, w.Code)
		}
	}
}
func TestStrictJSON(t *testing.T) {
	for _, body := range []string{`{"email":"a","extra":true}`, `{} {}`, `{`, strings.Repeat("x", (1<<20)+1)} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var target loginRequest
		if decodeJSON(httptest.NewRecorder(), r, &target) == nil {
			t.Errorf("accepted malformed JSON")
		}
	}
}
