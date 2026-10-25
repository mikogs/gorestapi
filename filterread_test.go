package gorestapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gorestapi/internal/fixtures"
)

// filterByFlags returns a FilterRead func that injects a Flags equality filter
// whose value is taken from the X-Owner-ID request header, mirroring the
// UserID ownership pattern used in the poc-api.
func filterReadByOwner() func(*http.Request) FilterSet {
	return func(r *http.Request) FilterSet {
		return FilterSet{Vals: map[string]string{"Flags": r.Header.Get("X-Owner-ID")}}
	}
}

// TestFilterRead_ReturnsMatchingRecord verifies that when the injected filter
// matches the stored record the handler returns the record normally.
func TestFilterRead_ReturnsMatchingRecord(t *testing.T) {
	recreateTestStructTable()

	ts := fixtures.TestStructWithData()
	ts.Flags = 7
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	req.Header.Set("X-Owner-ID", "7")
	w := httptest.NewRecorder()

	h := handlerWith(Route{FilterRead: filterReadByOwner()})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected %d, got %d", http.StatusOK, w.Code)
	}
}

// TestFilterRead_Returns404OnMismatch verifies that a filter mismatch returns
// 404 rather than 403, so callers cannot infer that the record exists.
func TestFilterRead_Returns404OnMismatch(t *testing.T) {
	recreateTestStructTable()

	ts := fixtures.TestStructWithData()
	ts.Flags = 7
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	req.Header.Set("X-Owner-ID", "99") // wrong owner
	w := httptest.NewRecorder()

	h := handlerWith(Route{FilterRead: filterReadByOwner()})
	h.Serve(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected %d, got %d — filter mismatch must not reveal record existence", http.StatusNotFound, w.Code)
	}
}
