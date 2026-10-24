package gorestapi

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
)

type HiddenStruct struct {
	ID       uint64 `json:"id"`
	Name     string `json:"name"`
	Secret   string `json:"-"`
	Password string `crud:"pass"`
}

func hiddenStructHandler(t *testing.T, route Route) *Handler {
	t.Helper()
	ctx := context.Background()

	_ = testCRUD.DropTable(ctx, &HiddenStruct{})
	if err := testCRUD.CreateTable(ctx, &HiddenStruct{}); err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}
	for _, obj := range []*HiddenStruct{{Name: "a", Secret: "x", Password: "p"}, {Name: "b", Secret: "y", Password: "p"}} {
		if err := testCRUD.Save(ctx, obj, gocrud.SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	reg := registry.New(map[string]func() any{"hidden": func() any { return &HiddenStruct{} }}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
	return New(reg, Options{Routes: map[string]Route{"hidden": route}})
}

func TestAPI_List_HiddenFields(t *testing.T) {
	h := hiddenStructHandler(t, Route{})

	for _, filter := range []string{"filter_val_Secret=x", "filter_val_Password=$2a$25&filter_op_Password=like"} {
		if got := listIDs(t, h, "/hidden/?order=ID&"+filter); !reflect.DeepEqual(got, []uint64{1, 2}) {
			t.Fatalf("%s on a field that is not returned returned %v, want it ignored", filter, got)
		}
	}
	if got := listIDs(t, h, "/hidden/?order=ID&filter_val_Name=b"); !reflect.DeepEqual(got, []uint64{2}) {
		t.Fatalf("a filter on a returned field returned %v, want [2]", got)
	}

	for _, field := range []string{"Secret", "Password", "Unknown"} {
		code, resp := serveJSON(t, h, http.MethodGet, "/hidden/?order="+field, 0, nil)
		if code != http.StatusBadRequest || !reflect.DeepEqual(resp.Data, map[string]any{field: "type"}) {
			t.Fatalf("a list ordered by %s returned %d %+v, want 400 with a type violation", field, code, resp)
		}
	}
}

func TestAPI_List_ListedHiddenFields(t *testing.T) {
	h := hiddenStructHandler(t, Route{AllowedFilters: []string{"Secret", "Password"}})

	if got := listIDs(t, h, "/hidden/?order=Secret&order_direction=desc&filter_val_Secret=y"); !reflect.DeepEqual(got, []uint64{2}) {
		t.Fatalf("a filter on a listed field that is not returned returned %v, want [2]", got)
	}

	code, resp := serveJSON(t, h, http.MethodGet, "/hidden/?filter_val_Password=$2a$25&filter_op_Password=like", 0, nil)
	if code != http.StatusBadRequest || !reflect.DeepEqual(resp.Data, map[string]any{"Password": gocrud.ViolationPassword}) {
		t.Fatalf("a filter on a listed password field returned %d %+v, want 400 with a password violation", code, resp)
	}
}
