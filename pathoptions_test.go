package gorestapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gorestapi/internal/fixtures"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

// handlerWith returns a new Handler using the shared testService but with the
// given Route applied to "teststruct".
func handlerWith(route Route) *Handler {
	return New(testService, Options{
		Routes: map[string]Route{
			"teststruct": route,
		},
	})
}

// ---------- Disabled operations ----------

func TestServe_DisableCreate(t *testing.T) {
	h := handlerWith(Route{Flags: DisableCreate})

	req := httptest.NewRequest(http.MethodPut, "/teststruct/", nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
	assertCode(t, w.Body.Bytes(), CodeNotAllowed)
}

func TestServe_DisableUpdate(t *testing.T) {
	h := handlerWith(Route{Flags: DisableUpdate})

	req := httptest.NewRequest(http.MethodPut, "/teststruct/1", nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
	assertCode(t, w.Body.Bytes(), CodeNotAllowed)
}

func TestServe_DisableDelete(t *testing.T) {
	h := handlerWith(Route{Flags: DisableDelete})

	req := httptest.NewRequest(http.MethodDelete, "/teststruct/1", nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
	assertCode(t, w.Body.Bytes(), CodeNotAllowed)
}

func TestServe_DisableRead(t *testing.T) {
	h := handlerWith(Route{Flags: DisableRead})

	req := httptest.NewRequest(http.MethodGet, "/teststruct/1", nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
	assertCode(t, w.Body.Bytes(), CodeNotAllowed)
}

func TestServe_DisableList(t *testing.T) {
	h := handlerWith(Route{Flags: DisableList})

	req := httptest.NewRequest(http.MethodGet, "/teststruct/", nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
	assertCode(t, w.Body.Bytes(), CodeNotAllowed)
}

// ---------- Filter constraints ----------

func TestServe_DisableFilters(t *testing.T) {
	recreateTestStructTable()

	// Insert rows with two distinct PrimaryEmail values.
	for i := 0; i < 3; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		ts.PrimaryEmail = "a@example.com"
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}
	for i := 0; i < 3; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		ts.PrimaryEmail = "b@example.com"
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	h := handlerWith(Route{Flags: DisableFilters})

	// Request with a filter that would normally narrow to 3 rows — should be ignored.
	url := "/teststruct/?limit=100&filter_val_PrimaryEmail=a@example.com&filter_op_PrimaryEmail=eq"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
	if count := responseDataLen(t, w.Body.Bytes()); count != 6 {
		t.Errorf("DisableFilters: expected all 6 rows, got %d", count)
	}
}

func TestServe_AllowedFilters_AllowedFieldIsApplied(t *testing.T) {
	recreateTestStructTable()

	for i := 0; i < 3; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		ts.PrimaryEmail = "a@example.com"
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}
	for i := 0; i < 3; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		ts.PrimaryEmail = "b@example.com"
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	h := handlerWith(Route{AllowedFilters: []string{"PrimaryEmail"}})

	url := "/teststruct/?limit=100&filter_val_PrimaryEmail=a@example.com&filter_op_PrimaryEmail=eq"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
	if count := responseDataLen(t, w.Body.Bytes()); count != 3 {
		t.Errorf("AllowedFilters: expected 3 rows for allowed filter, got %d", count)
	}
}

func TestServe_AllowedFilters_DisallowedFieldIsIgnored(t *testing.T) {
	recreateTestStructTable()

	for i := 0; i < 3; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		ts.PrimaryEmail = "a@example.com"
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}
	for i := 0; i < 3; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		ts.PrimaryEmail = "b@example.com"
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	// Only "Age" is allowed — PrimaryEmail filter should be dropped.
	h := handlerWith(Route{AllowedFilters: []string{"Age"}})

	url := "/teststruct/?limit=100&filter_val_PrimaryEmail=a@example.com&filter_op_PrimaryEmail=eq"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
	if count := responseDataLen(t, w.Body.Bytes()); count != 6 {
		t.Errorf("AllowedFilters: disallowed filter should be ignored, expected 6 rows, got %d", count)
	}
}

// ---------- Helpers ----------

func assertCode(t *testing.T, body []byte, want string) {
	t.Helper()
	var resp jsonresp.Response
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("could not unmarshal response: %v", err)
	}
	if resp.Code != want {
		t.Errorf("response code: want %q, got %q", want, resp.Code)
	}
}

func responseDataLen(t *testing.T, body []byte) int {
	t.Helper()
	var resp jsonresp.Response
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("could not unmarshal response: %v", err)
	}
	data, ok := resp.Data.([]any)
	if !ok {
		t.Fatalf("response data is not a slice")
	}
	return len(data)
}
