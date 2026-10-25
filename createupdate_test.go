package gorestapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikogs/gorestapi/internal/fixtures"
)

func TestServe_CreateUpdate(t *testing.T) {
	recreateTestStructTable()

	objJSON := fixtures.TestStructJSON()

	req := httptest.NewRequest(http.MethodPut, "/teststruct/", bytes.NewReader(objJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := resp["data"]
	if !ok {
		t.Fatalf("response does not contain 'data' field")
	}

	dataMap, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("data is not a map[string]any")
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

	objSaved := &fixtures.TestStruct{}
	if err := json.Unmarshal(objJSON, objSaved); err != nil {
		t.Fatalf("failed to unmarshal objJSON: %v", err)
	}

	objLoaded := fixtures.FatalIfNoTestStructInTheDatabase(t, testDB)
	if objLoaded.ID == 0 || !fixtures.AreTestStructObjectsSame(objSaved, objLoaded) {
		t.Fatalf("CreateUpdate failed to insert struct to the table")
	}

	objSaved.FirstName = "Updated"
	updateJSON, _ := json.Marshal(objSaved) // #nosec G117 -- test data, with the password of a test
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/teststruct/%d", idInt), bytes.NewReader(updateJSON))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	testHandler.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("an update returned %d %s, want %d", w.Code, w.Body.String(), http.StatusOK)
	}
	if objLoaded := fixtures.FatalIfNoTestStructInTheDatabase(t, testDB); objLoaded.ID != idInt || objLoaded.FirstName != "Updated" {
		t.Fatalf("an update stored %+v, want the first name of object %d updated", objLoaded, idInt)
	}
}
