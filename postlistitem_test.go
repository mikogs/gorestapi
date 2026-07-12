package gorestapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikolajgasior/gocrud"
	"github.com/mikolajgasior/gocrud/pkg/test"
	"github.com/mikolajgasior/gorestapi/pkg/jsonresp"
)

func TestPostListItem_MutatesListResponse(t *testing.T) {
	recreateTestStructTable()

	for i := 0; i < 3; i++ {
		ts := test.TestStructWithData()
		ts.ID = 0
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	req := httptest.NewRequest(http.MethodGet, "/teststruct/?limit=10", nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		PostListItem: func(obj interface{}, _ *http.Request) error {
			obj.(*test.TestStruct).FirstName = "ServerValue"
			return nil
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	var resp jsonresp.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	items := resp.Data.([]interface{})

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	for i, item := range items {
		itemBytes, _ := json.Marshal(item)
		var ts test.TestStruct
		json.Unmarshal(itemBytes, &ts)
		if ts.FirstName != "ServerValue" {
			t.Errorf("PostListItem: item %d: want FirstName %q, got %q", i, "ServerValue", ts.FirstName)
		}
	}
}

func TestPostListItem_DoesNotAffectSingleRead(t *testing.T) {
	recreateTestStructTable()

	ts := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		PostListItem: func(obj interface{}, _ *http.Request) error {
			obj.(*test.TestStruct).FirstName = "ServerValue"
			return nil
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	var resp jsonresp.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	dataBytes, _ := json.Marshal(resp.Data)
	var returned test.TestStruct
	json.Unmarshal(dataBytes, &returned)

	if returned.FirstName == "ServerValue" {
		t.Error("PostListItem must not fire on single-record read")
	}
}

func TestPostListItem_ErrorAbortsInList(t *testing.T) {
	recreateTestStructTable()

	ts := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, "/teststruct/?limit=10", nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		PostListItem: func(_ interface{}, _ *http.Request) error {
			return errors.New("list-item hook failure")
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected %d, got %d", http.StatusInternalServerError, w.Code)
	}
}
