package gorestapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/mikolajgasior/gocrud"
	"codeberg.org/mikolajgasior/gorestapi/pkg/jsonresp"
	"codeberg.org/mikolajgasior/gocrud/pkg/test"
)

func TestPostRead_MutatesReadResponse(t *testing.T) {
	recreateTestStructTable()

	ts := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		PostRead: func(obj interface{}, _ *http.Request) error {
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

	if returned.FirstName != "ServerValue" {
		t.Errorf("PostRead: want FirstName %q in response, got %q", "ServerValue", returned.FirstName)
	}
}

func TestPostRead_ErrorAbortsRead(t *testing.T) {
	recreateTestStructTable()

	ts := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		PostRead: func(_ interface{}, _ *http.Request) error {
			return errors.New("post-read failure")
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected %d, got %d", http.StatusInternalServerError, w.Code)
	}
}
