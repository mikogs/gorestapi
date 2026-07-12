package gorestapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikolajgasior/gocrud"
	"github.com/mikolajgasior/gocrud/pkg/test"
)

// insertTestStructsWithEmail inserts n records with the given PrimaryEmail.
func insertTestStructsWithEmail(n int, email string) {
	for i := 0; i < n; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		ts.PrimaryEmail = email
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}
}

// filterByEmail returns a FilterList func that always injects a PrimaryEmail
// equality filter with the given value.
func filterByEmail(email string) func(*http.Request) FilterSet {
	return func(_ *http.Request) FilterSet {
		return FilterSet{Vals: map[string]string{"PrimaryEmail": email}}
	}
}

func TestFilterList_InjectsFilter(t *testing.T) {
	recreateTestStructTable()

	insertTestStructsWithEmail(3, "owner@example.com")
	insertTestStructsWithEmail(3, "other@example.com")

	req := httptest.NewRequest(http.MethodGet, "/teststruct/?limit=100", nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{FilterList: filterByEmail("owner@example.com")})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
	if count := responseDataLen(t, w.Body.Bytes()); count != 3 {
		t.Errorf("expected 3 records matching injected filter, got %d", count)
	}
}

// Injected filters must take precedence over conflicting client-supplied filters
// so a caller cannot bypass server-side constraints.
func TestFilterList_OverridesClientFilter(t *testing.T) {
	recreateTestStructTable()

	insertTestStructsWithEmail(3, "owner@example.com")
	insertTestStructsWithEmail(3, "other@example.com")

	// Client tries to filter for "other", but the injected filter forces "owner".
	req := httptest.NewRequest(http.MethodGet, "/teststruct/?limit=100&filter_val_PrimaryEmail=other@example.com&filter_op_PrimaryEmail=eq", nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{FilterList: filterByEmail("owner@example.com")})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
	if count := responseDataLen(t, w.Body.Bytes()); count != 3 {
		t.Errorf("expected injected filter to win: want 3 owner rows, got %d", count)
	}
}

// FilterList must apply even when DisableFilters prevents client-supplied
// filters from being parsed.
func TestFilterList_WorksWithDisableFilters(t *testing.T) {
	recreateTestStructTable()

	insertTestStructsWithEmail(3, "owner@example.com")
	insertTestStructsWithEmail(3, "other@example.com")

	req := httptest.NewRequest(http.MethodGet, "/teststruct/?limit=100", nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		Flags:      DisableFilters,
		FilterList: filterByEmail("owner@example.com"),
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
	if count := responseDataLen(t, w.Body.Bytes()); count != 3 {
		t.Errorf("expected FilterList to apply despite DisableFilters: want 3, got %d", count)
	}
}
