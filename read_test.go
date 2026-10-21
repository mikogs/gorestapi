package gorestapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/test"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

func TestServe_Read(t *testing.T) {
	recreateTestStructTable()

	// Insert an object first
	objSaved := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), objSaved, gocrud.SaveOptions{})

	// Get object
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", objSaved.ID), nil)
	w := httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp jsonresp.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Data == nil {
		t.Error("expected Data field to be present in response")
	}

	// Verify Data field contains objSaved
	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("failed to marshal response data: %v", err)
	}

	var objReturned test.TestStruct
	err = json.Unmarshal(dataBytes, &objReturned)
	if err != nil {
		t.Fatalf("failed to unmarshal data to TestStruct: %v", err)
	}

	if objReturned.ID != objSaved.ID {
		t.Errorf("expected ID %d, got %d", objSaved.ID, objReturned.ID)
	}

	// Try to call a non-existing one
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", 3333), nil)
	w = httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}
