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
	req := httptest.NewRequest(http.MethodGet, "/teststruct/aaaa", nil)
	w := httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestServe_UnsupportedMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodPatch, "/teststruct/123", nil)
	w := httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
