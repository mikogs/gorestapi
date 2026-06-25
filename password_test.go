package gorestapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/mikolajgasior/gocrud"
	"codeberg.org/mikolajgasior/gocrud/pkg/service"
)

// PassStruct — no json tags on the password field.
// Table name: FieldToColumn("PassStruct") = "pass_struct"
type PassStruct struct {
	ID       uint64
	Name     string `crud:"req len:1,100"`
	Password string `crud:"pass"`
	Key      string `crud:"req uniq len:1,255"`
}

// PassStructJson — password field carries a json tag.
// "PassStructJson" has no underscore → table "pass_struct_json"
type PassStructJson struct {
	ID       uint64
	Name     string `crud:"req len:1,100"`
	Password string `crud:"pass" json:"password"`
	Key      string `crud:"req uniq len:1,255"`
}

// ── helpers ────────────────────────────────────────────────────────────────

func passStructService() (*service.CRUD, *Handler) {
	svc := service.New(map[string]func() interface{}{
		"passstruct": func() interface{} { return &PassStruct{} },
	}, testDB, gocrud.DialectPostgres)
	h := New(svc, Options{
		Routes: map[string]Route{
			"passstruct": {},
		},
	})
	return svc, h
}

func passStructJsonService() (*service.CRUD, *Handler) {
	svc := service.New(map[string]func() interface{}{
		"passstructjson": func() interface{} { return &PassStructJson{} },
	}, testDB, gocrud.DialectPostgres)
	h := New(svc, Options{
		Routes: map[string]Route{
			"passstructjson": {},
		},
	})
	return svc, h
}

func recreatePassStructTable() {
	_ = testCRUD.DropTable(context.Background(), &PassStruct{})
	_ = testCRUD.CreateTable(context.Background(), &PassStruct{})
}

func recreatePassStructJsonTable() {
	_ = testCRUD.DropTable(context.Background(), &PassStructJson{})
	_ = testCRUD.CreateTable(context.Background(), &PassStructJson{})
}

// responseKeys returns the set of keys present in a single-object Data field.
func responseKeys(t *testing.T, body []byte) map[string]bool {
	t.Helper()
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("responseKeys: unmarshal failed: %v", err)
	}
	keys := make(map[string]bool, len(resp.Data))
	for k := range resp.Data {
		keys[k] = true
	}
	return keys
}

// firstItemKeys returns the set of keys present on the first item in a list response.
func firstItemKeys(t *testing.T, body []byte) map[string]bool {
	t.Helper()
	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("firstItemKeys: unmarshal failed: %v", err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("firstItemKeys: data slice is empty")
	}
	keys := make(map[string]bool, len(resp.Data[0]))
	for k := range resp.Data[0] {
		keys[k] = true
	}
	return keys
}

// ── Read tests ─────────────────────────────────────────────────────────────

func TestAPI_Read_PasswordFieldAbsent(t *testing.T) {
	recreatePassStructTable()

	obj := &PassStruct{Name: "alice", Password: "secret", Key: "key-read-pass-absent-001"}
	_ = testCRUD.Save(context.Background(), obj, gocrud.SaveOptions{})

	_, h := passStructService()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/passstruct/%d", obj.ID), nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	keys := responseKeys(t, w.Body.Bytes())
	if keys["Password"] {
		t.Error("Read: password field (no json tag) should be absent from response")
	}
	if !keys["Name"] {
		t.Error("Read: Name field should be present in response")
	}
}

func TestAPI_Read_PasswordFieldAbsent_JSONTag(t *testing.T) {
	recreatePassStructJsonTable()

	obj := &PassStructJson{Name: "bob", Password: "hunter2", Key: "key-read-json-tag-001"}
	_ = testCRUD.Save(context.Background(), obj, gocrud.SaveOptions{})

	_, h := passStructJsonService()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/passstructjson/%d", obj.ID), nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	keys := responseKeys(t, w.Body.Bytes())
	if keys["password"] {
		t.Error("Read: password field (json:\"password\") should be absent from response")
	}
	if keys["Password"] {
		t.Error("Read: neither capitalised key should appear")
	}
	if !keys["Name"] {
		t.Error("Read: Name field should be present in response")
	}
}

// ── List tests ─────────────────────────────────────────────────────────────

func TestAPI_List_PasswordFieldAbsent(t *testing.T) {
	recreatePassStructTable()

	for i, name := range []string{"carol", "dave"} {
		obj := &PassStruct{Name: name, Password: "pw", Key: fmt.Sprintf("key-list-pass-absent-%03d", i)}
		_ = testCRUD.Save(context.Background(), obj, gocrud.SaveOptions{})
	}

	_, h := passStructService()
	req := httptest.NewRequest(http.MethodGet, "/passstruct/?limit=10", nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	keys := firstItemKeys(t, w.Body.Bytes())
	if keys["Password"] {
		t.Error("List: password field (no json tag) should be absent from response")
	}
	if !keys["Name"] {
		t.Error("List: Name field should be present in response")
	}
}

func TestAPI_List_PasswordFieldAbsent_JSONTag(t *testing.T) {
	recreatePassStructJsonTable()

	for i, name := range []string{"eve", "frank"} {
		obj := &PassStructJson{Name: name, Password: "pw", Key: fmt.Sprintf("key-list-json-tag-%03d", i)}
		_ = testCRUD.Save(context.Background(), obj, gocrud.SaveOptions{})
	}

	_, h := passStructJsonService()
	req := httptest.NewRequest(http.MethodGet, "/passstructjson/?limit=10", nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	keys := firstItemKeys(t, w.Body.Bytes())
	if keys["password"] {
		t.Error("List: password field (json:\"password\") should be absent from response")
	}
	if keys["Password"] {
		t.Error("List: neither capitalised key should appear")
	}
	if !keys["Name"] {
		t.Error("List: Name field should be present in response")
	}
}

// ── No password field — baseline ───────────────────────────────────────────

func TestAPI_Read_NoPasswordField_UnaffectedResponse(t *testing.T) {
	recreatePassStructTable()

	// Save a struct without a pass tag via the JSON body path to confirm
	// that structs with no password fields still serialise normally.
	body, _ := json.Marshal(map[string]interface{}{
		"Name": "grace",
		"Key":  "key-no-pass-baseline-001",
	})
	_, h := passStructService()
	reqCreate := httptest.NewRequest(http.MethodPut, "/passstruct/", bytes.NewReader(body))
	wCreate := httptest.NewRecorder()
	h.Serve(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", wCreate.Code, wCreate.Body.String())
	}

	var created struct {
		Data struct{ ID float64 } `json:"data"`
	}
	json.Unmarshal(wCreate.Body.Bytes(), &created)
	id := uint64(created.Data.ID)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/passstruct/%d", id), nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	keys := responseKeys(t, w.Body.Bytes())
	if !keys["Name"] || !keys["ID"] {
		t.Errorf("baseline: expected normal fields to be present, got keys: %v", keys)
	}
}
