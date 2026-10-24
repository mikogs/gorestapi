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

func TestServe_Delete(t *testing.T) {
	recreateTestStructTable()

	// Insert an object first
	objSaved := fixtures.TestStructWithData()
	_ = testCRUD.Save(context.Background(), objSaved, gocrud.SaveOptions{})

	// Delete it
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/teststruct/%d", objSaved.ID), nil)
	w := httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	fixtures.FatalIfTestStructNotDeletedInTheDatabase(t, testDB, objSaved.ID)

	// Try to delete it again - should return NotFound
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/teststruct/%d", objSaved.ID), nil)
	w = httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}
