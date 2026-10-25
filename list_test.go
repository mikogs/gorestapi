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

func TestServe_List(t *testing.T) {
	recreateTestStructTable()

	// Insert some data that should be ignored by Get later on
	for i := 1; i < 51; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		ts.Age = 10 + i
		ts.Price = 444
		ts.PrimaryEmail = "another@example.com"
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	// Insert data that should be selected by filters
	for i := 1; i < 51; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		ts.Age = 30 + i
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	urlPath := "/teststruct/?limit=10&offset=20&order=Age&order_direction=asc&filter_val_Price=444&filter_op_Price=eq&filter_val_PrimaryEmail=primary@example.com&filter_op_PrimaryEmail=eq"

	req := httptest.NewRequest(http.MethodGet, urlPath, nil)
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

	dataSlice, ok := resp.Data.([]any)
	if !ok {
		t.Fatal("expected Data to be a slice")
	}

	if len(dataSlice) != 10 {
		t.Errorf("expected 10 items in Data, got %d", len(dataSlice))
	}

	dataBytes, err := json.Marshal(dataSlice[2])
	if err != nil {
		t.Fatalf("failed to marshal item at index 2: %v", err)
	}

	var objReturned fixtures.TestStruct
	err = json.Unmarshal(dataBytes, &objReturned)
	if err != nil {
		t.Fatalf("failed to unmarshal item at index 2 to TestStruct: %v", err)
	}

	if objReturned.Age != 53 {
		t.Errorf("expected Age to be 53, got %d", objReturned.Age)
	}
}
