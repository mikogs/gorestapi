package gorestapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	w := httptest.NewRecorder()
	Liveness(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("Liveness returned %d, want 200", w.Code)
	}

	h := New(testService, Options{})
	w = httptest.NewRecorder()
	h.Readiness(testDB.PingContext).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("Readiness with a database returned %d, want 200", w.Code)
	}

	failing := func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("the check has a context without the timeout")
		}
		return errors.New("no database")
	}
	w, problem := serveProblem(t, h.Readiness(failing), http.MethodGet, "/readyz", "")
	if w.Code != http.StatusServiceUnavailable || problem["code"] != CodeServiceUnavailable || problem["instance"] != "/readyz" {
		t.Fatalf("Readiness with a failing check returned %d %v, want the problem SERVICE_UNAVAILABLE", w.Code, problem)
	}
}
