package gorestapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gorestapi/internal/fixtures"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

// Minimal struct variants that map to test_struct via the existing "_" name-stripping:
//   strings.Split("TestStruct_X", "_")[0] = "TestStruct" → "test_struct"

type TestStruct_Create struct {
	ID  uint64
	Key string `crud:"uniq" validate:"required,min=30,max=255"`
}

type TestStruct_Update struct {
	ID        uint64
	FirstName string `validate:"required,min=2,max=30"`
}

type TestStruct_ReadOnly struct {
	ID           uint64
	PrimaryEmail string
}

type TestStruct_ListOnly struct {
	ID    uint64
	Price int
}

// ---------- Create ----------

func TestServe_CreateConstructor(t *testing.T) {
	recreateTestStructTable()

	key := "uniquekey-create-constructor-test-abc123"

	body, _ := json.Marshal(map[string]any{"Key": key})
	req := httptest.NewRequest(http.MethodPut, "/teststruct/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		CreateConstructor: func() any { return &TestStruct_Create{} },
	})
	h.Serve(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d — body: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp jsonresp.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %s", err.Error())
	}
	data := resp.Data.(map[string]any)
	id := uint64(data["id"].(float64))
	if id == 0 {
		t.Fatalf("CreateConstructor: expected non-zero id in response")
	}

	// Verify the row was actually inserted in the DB.
	var count int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM test_struct WHERE id=$1 AND key=$2", id, key).Scan(&count); err != nil {
		t.Fatalf("query failed: %s", err.Error())
	}
	if count != 1 {
		t.Fatalf("CreateConstructor: record not found in DB (id=%d key=%s)", id, key)
	}
}

// ---------- Update ----------

func TestServe_UpdateConstructor(t *testing.T) {
	recreateTestStructTable()

	// Insert a full record first.
	original := fixtures.TestStructWithData()
	_ = testCRUD.Save(context.Background(), original, gocrud.SaveOptions{})

	body, _ := json.Marshal(map[string]any{"FirstName": "UpdatedViaOverride"})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/teststruct/%d", original.ID), bytes.NewReader(body))
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		UpdateConstructor: func() any { return &TestStruct_Update{} },
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d — body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	// Verify FirstName was updated and PrimaryEmail was left untouched.
	var firstName, primaryEmail string
	err := testDB.QueryRow("SELECT first_name, primary_email FROM test_struct WHERE id=$1", original.ID).
		Scan(&firstName, &primaryEmail)
	if err != nil {
		t.Fatalf("query failed: %s", err.Error())
	}

	if firstName != "UpdatedViaOverride" {
		t.Errorf("UpdateConstructor: FirstName want %q, got %q", "UpdatedViaOverride", firstName)
	}
	if primaryEmail != original.PrimaryEmail {
		t.Errorf("UpdateConstructor: PrimaryEmail should be unchanged, want %q, got %q", original.PrimaryEmail, primaryEmail)
	}
}

// ---------- Read ----------

func TestServe_ReadConstructor(t *testing.T) {
	recreateTestStructTable()

	inserted := fixtures.TestStructWithData()
	_ = testCRUD.Save(context.Background(), inserted, gocrud.SaveOptions{})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/teststruct/%d", inserted.ID), nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		ReadConstructor: func() any { return &TestStruct_ReadOnly{} },
	})
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	// Unmarshal as a generic map to inspect which fields came back.
	var resp jsonresp.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %s", err.Error())
	}
	dataBytes, _ := json.Marshal(resp.Data)
	var item map[string]any
	if err := json.Unmarshal(dataBytes, &item); err != nil {
		t.Fatalf("unmarshal failed: %s", err.Error())
	}

	if _, ok := item["ID"]; !ok {
		t.Errorf("ReadConstructor: expected ID field in response")
	}
	if _, ok := item["PrimaryEmail"]; !ok {
		t.Errorf("ReadConstructor: expected PrimaryEmail field in response")
	}
	if _, ok := item["FirstName"]; ok {
		t.Errorf("ReadConstructor: unexpected FirstName field — override struct should not include it")
	}
}

// ---------- List ----------

func TestServe_ListConstructor(t *testing.T) {
	recreateTestStructTable()

	for i := 0; i < 3; i++ {
		ts := fixtures.TestStructWithData()
		ts.ID = 0
		ts.Price = 100 + i
		_ = testCRUD.Save(context.Background(), ts, gocrud.SaveOptions{})
	}

	req := httptest.NewRequest(http.MethodGet, "/teststruct/?limit=10", nil)
	w := httptest.NewRecorder()

	h := handlerWith(Route{
		ListConstructor: func() any { return &TestStruct_ListOnly{} },
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
		t.Fatalf("ListConstructor: expected 3 items, got %d", len(items))
	}

	// Each item should only have the fields from TestStruct_ListOnly.
	itemBytes, _ := json.Marshal(items[0])
	var item map[string]any
	if err := json.Unmarshal(itemBytes, &item); err != nil {
		t.Fatalf("unmarshal failed: %s", err.Error())
	}

	if _, ok := item["ID"]; !ok {
		t.Errorf("ListConstructor: expected ID field")
	}
	if _, ok := item["Price"]; !ok {
		t.Errorf("ListConstructor: expected Price field")
	}
	if _, ok := item["PrimaryEmail"]; ok {
		t.Errorf("ListConstructor: unexpected PrimaryEmail field — override struct should not include it")
	}
}
