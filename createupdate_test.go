package gorestapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikogs/gocrud/pkg/test"
)

func TestServe_CreateUpdate(t *testing.T) {
	recreateTestStructTable()

	objJSON := test.TestStructJSON()

	req := httptest.NewRequest(http.MethodPut, "/teststruct/", bytes.NewReader(objJSON))
	w := httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := resp["data"]
	if !ok {
		t.Fatalf("response does not contain 'data' field")
	}

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not a map[string]interface{}")
	}

	idVal, ok := dataMap["id"]
	if !ok {
		t.Fatalf("data does not contain 'id' key")
	}

	idFloat, ok := idVal.(float64)
	if !ok {
		t.Fatalf("id value is not a number")
	}

	idInt := uint64(idFloat)
	if idInt <= 0 {
		t.Fatalf("id value is not greater than 0, got %d", idInt)
	}

	objSaved := &test.TestStruct{}
	if err := json.Unmarshal(objJSON, objSaved); err != nil {
		t.Fatalf("failed to unmarshal objJSON: %v", err)
	}

	objLoaded := test.FatalIfNoTestStructInTheDatabase(t, testDB)
	if objLoaded.ID == 0 || !test.AreTestStructObjectsSame(objSaved, objLoaded) {
		t.Fatalf("CreateUpdate failed to insert struct to the table")
	}

	objSaved.FirstName = "Updated"
	updateJSON, _ := json.Marshal(objSaved)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/teststruct/%d", idInt), bytes.NewReader(updateJSON))
	w = httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("an update returned %d %s, want %d", w.Code, w.Body.String(), http.StatusOK)
	}
	if objLoaded := test.FatalIfNoTestStructInTheDatabase(t, testDB); objLoaded.ID != idInt || objLoaded.FirstName != "Updated" {
		t.Fatalf("an update stored %+v, want the first name of object %d updated", objLoaded, idInt)
	}
}
