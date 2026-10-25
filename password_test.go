package gorestapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
)

// PassStruct — no json tags on the password field.
// Table name: FieldToColumn("PassStruct") = "pass_struct"
type PassStruct struct {
	ID       uint64
	Name     string `validate:"required,min=1,max=100"`
	Password string `crud:"pass"`
	Key      string `crud:"uniq" validate:"required,min=1,max=255"`
}

// PassStructJson — password field carries a json tag.
// "PassStructJson" has no underscore → table "pass_struct_json"
type PassStructJson struct {
	ID       uint64
	Name     string `validate:"required,min=1,max=100"`
	Password string `crud:"pass" json:"password"`
	Key      string `crud:"uniq" validate:"required,min=1,max=255"`
}

// ── helpers ────────────────────────────────────────────────────────────────

func passStructService() (*registry.CRUD, *Handler) {
	reg := registry.New(map[string]func() any{
		"passstruct": func() any { return &PassStruct{} },
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
	h := New(reg, Options{
		Routes: map[string]Route{
			"passstruct": {},
		},
	})
	return reg, h
}

func passStructJsonService() (*registry.CRUD, *Handler) {
	reg := registry.New(map[string]func() any{
		"passstructjson": func() any { return &PassStructJson{} },
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
	h := New(reg, Options{
		Routes: map[string]Route{
			"passstructjson": {},
		},
	})
	return reg, h
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
		Data map[string]any `json:"data"`
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
		Data []map[string]any `json:"data"`
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
	body, _ := json.Marshal(map[string]any{
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
	if err := json.Unmarshal(wCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal failed: %s", err.Error())
	}
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

func TestAPI_Update_Password(t *testing.T) {
	recreatePassStructTable()
	reg, h := passStructService()
	ctx := context.Background()

	obj := &PassStruct{Name: "alice", Password: "secret", Key: "k1"}
	if err := reg.Save(ctx, obj, nil); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	path := fmt.Sprintf("/passstruct/%d", obj.ID)

	verify := func(password string, want bool) {
		t.Helper()
		_, verified, err := reg.VerifyPasswords(ctx, "passstruct", registry.Filters{Vals: map[string]string{"ID": strconv.FormatUint(obj.ID, 10)}},
			map[string]string{"Password": password})
		if err != nil || verified != want {
			t.Fatalf("verification of %q returned %t, %v, want %t", password, verified, err, want)
		}
	}

	if code, resp := serveJSON(t, h, http.MethodPut, path, 0, map[string]string{"Name": "alicia", "Key": "k1"}); code != http.StatusOK {
		t.Fatalf("PUT without a password returned %d %+v", code, resp)
	}
	verify("secret", true)
	verify("", false)

	if code, resp := serveJSON(t, h, http.MethodPut, path, 0, map[string]string{"Name": "alicia", "Key": "k1", "Password": "changed"}); code != http.StatusOK {
		t.Fatalf("PUT with a password returned %d %+v", code, resp)
	}
	verify("changed", true)
	verify("secret", false)
}

func TestAPI_Create_PasswordTooLong(t *testing.T) {
	recreatePassStructTable()
	_, h := passStructService()

	code, resp := serveJSON(t, h, http.MethodPut, "/passstruct/", 0, map[string]string{"Name": "alice", "Key": "k1", "Password": strings.Repeat("x", 73)})
	if code != http.StatusBadRequest || !reflect.DeepEqual(resp.Errors, map[string]string{"Password": "max"}) {
		t.Fatalf("PUT with a password that is too long returned %d %+v, want 400 with a max violation", code, resp)
	}
}
