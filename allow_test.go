package gorestapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikolajgasior/gocrud"
	"github.com/mikolajgasior/gocrud/pkg/test"
)

// ---------- AllowCreate ----------

func TestAllow_Create_Allows(t *testing.T) {
	recreateTestStructTable()

	body, _ := json.Marshal(map[string]interface{}{"Key": "allow-create-allows-00000000000000000"})
	req := httptest.NewRequest(http.MethodPut, "/teststruct/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		CreateConstructor: func() interface{} { return &TestStruct_Create{} },
		AllowCreate:       func(_ interface{}, _ *http.Request) error { return nil },
	})
	h.Serve(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected %d, got %d", http.StatusCreated, w.Code)
	}
}

func TestAllow_Create_Rejects(t *testing.T) {
	recreateTestStructTable()

	body, _ := json.Marshal(map[string]interface{}{"Key": "allow-create-rejects-0000000000000000"})
	req := httptest.NewRequest(http.MethodPut, "/teststruct/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		CreateConstructor: func() interface{} { return &TestStruct_Create{} },
		AllowCreate:       func(_ interface{}, _ *http.Request) error { return errors.New("forbidden") },
	})
	h.Serve(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected %d, got %d", http.StatusForbidden, w.Code)
	}
	assertCode(t, w.Body.Bytes(), CodeForbidden)
}

// ---------- AllowUpdate ----------

func TestAllow_Update_Allows(t *testing.T) {
	recreateTestStructTable()

	original := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), original, gocrud.SaveOptions{})

	body, _ := json.Marshal(map[string]interface{}{"FirstName": "Updated"})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/teststruct/%d", original.ID), bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		UpdateConstructor: func() interface{} { return &TestStruct_Update{} },
		AllowUpdate:       func(_ interface{}, _ *http.Request) error { return nil },
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected %d, got %d", http.StatusOK, w.Code)
	}
}

func TestAllow_Update_Rejects(t *testing.T) {
	recreateTestStructTable()

	original := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), original, gocrud.SaveOptions{})

	body, _ := json.Marshal(map[string]interface{}{"FirstName": "Updated"})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/teststruct/%d", original.ID), bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		UpdateConstructor: func() interface{} { return &TestStruct_Update{} },
		AllowUpdate:       func(_ interface{}, _ *http.Request) error { return errors.New("forbidden") },
	})
	h.Serve(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected %d, got %d", http.StatusForbidden, w.Code)
	}
	assertCode(t, w.Body.Bytes(), CodeForbidden)
}

// AllowUpdate must receive the stored record, not the values from the
// incoming request body.
func TestAllow_Update_ReceivesStoredRecord(t *testing.T) {
	recreateTestStructTable()

	original := test.TestStructWithData()
	original.FirstName = "StoredName"
	_ = testCRUD.Save(context.Background(), original, gocrud.SaveOptions{})

	var seenFirstName string
	body, _ := json.Marshal(map[string]interface{}{"FirstName": "IncomingName"})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/teststruct/%d", original.ID), bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		UpdateConstructor: func() interface{} { return &TestStruct_Update{} },
		AllowUpdate: func(obj interface{}, _ *http.Request) error {
			if u, ok := obj.(*TestStruct_Update); ok {
				seenFirstName = u.FirstName
			}
			return nil
		},
	})
	h.Serve(w, req)

	if seenFirstName != "StoredName" {
		t.Errorf("AllowUpdate: want stored FirstName %q, got %q", "StoredName", seenFirstName)
	}
}

// ---------- AllowRead ----------

func TestAllow_Read_Allows(t *testing.T) {
	recreateTestStructTable()

	ts := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		AllowRead: func(_ interface{}, _ *http.Request) error { return nil },
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected %d, got %d", http.StatusOK, w.Code)
	}
}

func TestAllow_Read_Rejects(t *testing.T) {
	recreateTestStructTable()

	ts := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		AllowRead: func(_ interface{}, _ *http.Request) error { return errors.New("forbidden") },
	})
	h.Serve(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected %d, got %d", http.StatusForbidden, w.Code)
	}
	assertCode(t, w.Body.Bytes(), CodeForbidden)
}

// ---------- AllowDelete ----------

func TestAllow_Delete_Allows(t *testing.T) {
	recreateTestStructTable()

	ts := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		AllowDelete: func(_ interface{}, _ *http.Request) error { return nil },
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected %d, got %d", http.StatusOK, w.Code)
	}
}

func TestAllow_Delete_Rejects(t *testing.T) {
	recreateTestStructTable()

	ts := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/teststruct/%d", ts.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		AllowDelete: func(_ interface{}, _ *http.Request) error { return errors.New("forbidden") },
	})
	h.Serve(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected %d, got %d", http.StatusForbidden, w.Code)
	}
	assertCode(t, w.Body.Bytes(), CodeForbidden)
}
