package gorestapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSecurityHeaders(t *testing.T) {
	want := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Content-Security-Policy":   "default-src 'none'; frame-ancestors 'none'",
		"Referrer-Policy":           "no-referrer",
		"Cache-Control":             "no-store",
		"Strict-Transport-Security": "",
	}
	for name, test := range map[string]struct {
		options Options
		want    map[string]string
	}{
		"default": {Options{}, want},
		"public data, over HTTPS": {Options{CacheControl: "public, max-age=60", HSTSMaxAge: 365 * 24 * time.Hour}, map[string]string{
			"Cache-Control": "public, max-age=60", "Strict-Transport-Security": "max-age=31536000; includeSubDomains", "X-Frame-Options": "DENY",
		}},
		"disabled": {Options{DisableSecurityHeaders: true}, map[string]string{"X-Frame-Options": "", "Cache-Control": ""}},
	} {
		test.options.Routes = map[string]Route{"teststruct": {}}
		h := New(testService, test.options)
		// also the problems, e.g. of a route that does not exist
		for _, path := range []string{"/teststruct/", "/unknown/"} {
			w := httptest.NewRecorder()
			h.Serve(w, httptest.NewRequest(http.MethodGet, path, nil))
			for header, value := range test.want {
				if got := w.Header().Get(header); got != value {
					t.Fatalf("%s: GET %s returned %s: %q, want %q", name, path, header, got, value)
				}
			}
		}
	}
}
