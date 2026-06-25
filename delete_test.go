package gorestapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/mikolajgasior/gocrud"
	"codeberg.org/mikolajgasior/gocrud/pkg/test"
)

func TestServe_Delete(t *testing.T) {
	recreateTestStructTable()

	// Insert an object first
	objSaved := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), objSaved, gocrud.SaveOptions{})

	// Delete it
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/teststruct/%d", objSaved.ID), nil)
	w := httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	test.FatalIfTestStructNotDeletedInTheDatabase(t, testDB, objSaved.ID)

	// Try to delete it again - should return NotFound
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/teststruct/%d", objSaved.ID), nil)
	w = httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}
