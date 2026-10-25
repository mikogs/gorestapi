package gorestapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gorestapi/internal/fixtures"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

func TestPostListItem_MutatesListResponse(t *testing.T) {
	recreateTestStructTable()

	for i := 0; i < 3; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	req := httptest.NewRequest(http.MethodGet, "/teststruct/?limit=10", nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		PostListItem: func(obj any, _ *http.Request) error {
			obj.(*fixtures.TestStruct).FirstName = "ServerValue"
			return nil
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	var resp jsonresp.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %s", err.Error())
	}
	items := resp.Data.([]any)

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	for i, item := range items {
		itemBytes, _ := json.Marshal(item)
		var ts fixtures.TestStruct
		if err := json.Unmarshal(itemBytes, &ts); err != nil {
			t.Fatalf("unmarshal failed: %s", err.Error())
		}
		if ts.FirstName != "ServerValue" {
			t.Errorf("PostListItem: item %d: want FirstName %q, got %q", i, "ServerValue", ts.FirstName)
		}
	}
}

func TestPostListItem_DoesNotAffectSingleRead(t *testing.T) {
	recreateTestStructTable()

	ts := fixtures.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		PostListItem: func(obj any, _ *http.Request) error {
			obj.(*fixtures.TestStruct).FirstName = "ServerValue"
			return nil
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	var resp jsonresp.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %s", err.Error())
	}
	dataBytes, _ := json.Marshal(resp.Data)
	var returned fixtures.TestStruct
	if err := json.Unmarshal(dataBytes, &returned); err != nil {
		t.Fatalf("unmarshal failed: %s", err.Error())
	}

	if returned.FirstName == "ServerValue" {
		t.Error("PostListItem must not fire on single-record read")
	}
}

func TestPostListItem_ErrorAbortsInList(t *testing.T) {
	recreateTestStructTable()

	ts := fixtures.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, "/teststruct/?limit=10", nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		PostListItem: func(_ any, _ *http.Request) error {
			return errors.New("list-item hook failure")
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected %d, got %d", http.StatusInternalServerError, w.Code)
	}
}
