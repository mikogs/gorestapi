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

	"codeberg.org/mikolajgasior/gocrud"
	"codeberg.org/mikolajgasior/gocrud/pkg/test"
)

// ---------- PreCreate ----------

func TestPreCreate_MutatesObject(t *testing.T) {
	recreateTestStructTable()

	clientKey := "precreate-client-key-000000000000000"
	serverKey := "precreate-server-key-000000000000000"

	body, _ := json.Marshal(map[string]interface{}{"Key": clientKey})
	req := httptest.NewRequest(http.MethodPut, "/teststruct/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		CreateConstructor: func() interface{} { return &TestStruct_Create{} },
		PreCreate: func(obj interface{}, _ *http.Request) error {
			obj.(*TestStruct_Create).Key = serverKey
			return nil
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d", http.StatusCreated, w.Code)
	}

	var count int
	testDB.QueryRow("SELECT COUNT(*) FROM test_struct WHERE key = $1", serverKey).Scan(&count)
	if count != 1 {
		t.Errorf("PreCreate: server key %q not found in DB — hook did not mutate the object", serverKey)
	}

	testDB.QueryRow("SELECT COUNT(*) FROM test_struct WHERE key = $1", clientKey).Scan(&count)
	if count != 0 {
		t.Errorf("PreCreate: client key %q should have been overridden", clientKey)
	}
}

func TestPreCreate_ErrorAbortsCreate(t *testing.T) {
	recreateTestStructTable()

	body, _ := json.Marshal(map[string]interface{}{"Key": "precreate-error-key-000000000000000"})
	req := httptest.NewRequest(http.MethodPut, "/teststruct/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		CreateConstructor: func() interface{} { return &TestStruct_Create{} },
		PreCreate: func(_ interface{}, _ *http.Request) error {
			return errors.New("pre-create failure")
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

// ---------- PreUpdate ----------

func TestPreUpdate_MutatesObject(t *testing.T) {
	recreateTestStructTable()

	original := test.TestStructWithData()
	original.FirstName = "Before"
	_ = testCRUD.Save(context.Background(), original, gocrud.SaveOptions{})

	body, _ := json.Marshal(map[string]interface{}{"FirstName": "FromClient"})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/teststruct/%d", original.ID), bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		UpdateConstructor: func() interface{} { return &TestStruct_Update{} },
		PreUpdate: func(obj interface{}, _ *http.Request) error {
			obj.(*TestStruct_Update).FirstName = "FromServer"
			return nil
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	var saved string
	testDB.QueryRow("SELECT first_name FROM test_struct WHERE id = $1", original.ID).Scan(&saved)
	if saved != "FromServer" {
		t.Errorf("PreUpdate: want %q in DB, got %q — hook did not mutate the object", "FromServer", saved)
	}
}

func TestPreUpdate_ErrorAbortsUpdate(t *testing.T) {
	recreateTestStructTable()

	original := test.TestStructWithData()
	_ = testCRUD.Save(context.Background(), original, gocrud.SaveOptions{})

	body, _ := json.Marshal(map[string]interface{}{"FirstName": "Updated"})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/teststruct/%d", original.ID), bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		UpdateConstructor: func() interface{} { return &TestStruct_Update{} },
		PreUpdate: func(_ interface{}, _ *http.Request) error {
			return errors.New("pre-update failure")
		},
	})
	h.Serve(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected %d, got %d", http.StatusInternalServerError, w.Code)
	}
}
