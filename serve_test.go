package gorestapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServe_InvalidPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/invalidpath/123", nil)
	w := httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestServe_InvalidID(t *testing.T) {
	tests := map[string]int{
		"/teststruct/aaaa":  http.StatusNotFound,
		"/teststruct/1/2":   http.StatusBadRequest,
		"/teststruct/1;del": http.StatusBadRequest,
	}

	for path, want := range tests {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			w := httptest.NewRecorder()
			testHandler.Serve(w, httptest.NewRequest(method, path, nil))
			if w.Code != want {
				t.Errorf("%s %s returned %d, want %d", method, path, w.Code, want)
			}
		}
	}
}

func TestServe_UnsupportedMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodPatch, "/teststruct/123", nil)
	w := httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "OPTIONS, GET, PUT, DELETE" {
		t.Errorf("PATCH returned %d with Allow %q, want 405 with the methods of a record", w.Code, w.Header().Get("Allow"))
	}
}
