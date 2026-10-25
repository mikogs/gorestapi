package gorestapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/cors"
)

func TestOptions(t *testing.T) {
	h := New(testService, Options{Routes: map[string]Route{"teststruct": {Flags: DisableDelete}}})
	for path, want := range map[string]struct {
		status int
		allow  string
	}{
		"/teststruct/":  {http.StatusNoContent, "OPTIONS, GET, PUT"},
		"/teststruct/1": {http.StatusNoContent, "OPTIONS, GET, PUT"},
		"/unknown/":     {http.StatusNotFound, ""},
	} {
		w := httptest.NewRecorder()
		h.Serve(w, httptest.NewRequest(http.MethodOptions, path, nil))
		if w.Code != want.status || w.Header().Get("Allow") != want.allow {
			t.Fatalf("OPTIONS %s returned %d with Allow %q, want %d with %q", path, w.Code, w.Header().Get("Allow"), want.status, want.allow)
		}
	}
}

// TestCORS checks the configuration of rs/cors of the docs, in front of the handler.
func TestCORS(t *testing.T) {
	h := New(testService, Options{Routes: map[string]Route{"teststruct": {}}})
	handler := cors.New(cors.Options{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowedMethods:   []string{http.MethodGet, http.MethodPut, http.MethodDelete},
		AllowedHeaders:   []string{"Authorization", "Content-Type", RequestIDHeader},
		ExposedHeaders:   []string{RequestIDHeader, "Allow"},
		AllowCredentials: false,
		MaxAge:           600,
	}).Handler(http.HandlerFunc(h.Serve))

	preflight := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodOptions, "/teststruct/1", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", http.MethodPut)
		req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	if w := preflight("https://app.example.com"); w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" ||
		w.Header().Get("Access-Control-Allow-Methods") != http.MethodPut {
		t.Fatalf("the preflight of the app returned %d %v, want the origin and the method allowed", w.Code, w.Header())
	}
	if w := preflight("https://evil.example.com"); w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("the preflight of another site returned %v, want no CORS headers", w.Header())
	}

	req := httptest.NewRequest(http.MethodGet, "/teststruct/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" || w.Header().Get("Access-Control-Expose-Headers") == "" ||
		w.Header().Get("Vary") == "" {
		t.Fatalf("a request of the app returned %v, want its origin, the exposed headers and Vary", w.Header())
	}
}
