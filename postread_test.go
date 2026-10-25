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

func TestPostRead_MutatesReadResponse(t *testing.T) {
	recreateTestStructTable()

	ts := fixtures.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		PostRead: func(obj any, _ *http.Request) error {
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

	if returned.FirstName != "ServerValue" {
		t.Errorf("PostRead: want FirstName %q in response, got %q", "ServerValue", returned.FirstName)
	}
}

func TestPostRead_ErrorAbortsRead(t *testing.T) {
	recreateTestStructTable()

	ts := fixtures.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		PostRead: func(_ any, _ *http.Request) error {
			return errors.New("post-read failure")
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected %d, got %d", http.StatusInternalServerError, w.Code)
	}
}
