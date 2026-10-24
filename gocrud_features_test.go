package gorestapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi/internal/fixtures"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

type ApiCustomer struct {
	ID         uint64    `json:"id"`
	Name       string    `json:"name" validate:"required"`
	CreatedAt  time.Time `json:"created_at"`
	CreatedBy  uint64    `json:"created_by"`
	ModifiedAt time.Time `json:"modified_at"`
	ModifiedBy uint64    `json:"modified_by"`
}

type ApiOrder struct {
	ID         uint64       `json:"id"`
	CustomerID uint64       `json:"customer_id" crud:"fk:api_customer"`
	Status     string       `json:"status" validate:"oneof=new paid shipped"`
	ShippedAt  *time.Time   `json:"shipped_at"`
	Customer   *ApiCustomer `json:"customer,omitempty" crud:"rel:CustomerID"`
}

func apiHandler(t *testing.T) *Handler {
	t.Helper()
	ctx := context.Background()

	_ = testCRUD.DropTable(ctx, &ApiOrder{})
	_ = testCRUD.DropTable(ctx, &ApiCustomer{})

	reg := registry.New(map[string]func() any{
		"customers": func() any { return &ApiCustomer{} },
		"orders":    func() any { return &ApiOrder{} },
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
	if err := reg.CreateTables(ctx); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}

	return New(reg, Options{
		Routes: map[string]Route{"customers": {}, "orders": {}},
		UserIDFunc: func(r *http.Request) any {
			id, _ := strconv.ParseUint(r.Header.Get("X-User-ID"), 10, 64)
			return id
		},
	})
}

func serveJSON(t *testing.T, h *Handler, method string, path string, userID uint64, body any) (int, jsonresp.Response) {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("X-User-ID", strconv.FormatUint(userID, 10))
	w := httptest.NewRecorder()
	h.Serve(w, req)

	var resp jsonresp.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response %q: %v", w.Body.String(), err)
	}

	return w.Code, resp
}

func listIDs(t *testing.T, h *Handler, path string) []uint64 {
	t.Helper()

	code, resp := serveJSON(t, h, http.MethodGet, path, 0, nil)
	if code != http.StatusOK {
		t.Fatalf("GET %s returned %d %+v", path, code, resp)
	}

	var ids []uint64
	for _, item := range resp.Data.([]any) {
		ids = append(ids, uint64(item.(map[string]any)["id"].(float64)))
	}
	return ids
}

func TestServe_ValidationViolations(t *testing.T) {
	h := apiHandler(t)

	code, resp := serveJSON(t, h, http.MethodPut, "/orders/", 1, map[string]any{"customer_id": 1, "status": "lost"})
	if code != http.StatusBadRequest || resp.Code != jsonresp.CodeValidationFailed || resp.Message == nil || *resp.Message != "invalid object" {
		t.Fatalf("got %d %+v, want %d %s with the message invalid object", code, resp, http.StatusBadRequest, jsonresp.CodeValidationFailed)
	}
	if !reflect.DeepEqual(resp.Data, map[string]any{"Status": "oneof"}) {
		t.Fatalf("got data %v, want the violations {Status: oneof}", resp.Data)
	}
}

func TestServe_ValidationMessages(t *testing.T) {
	h := apiHandler(t)
	h.options.Routes["orders"] = Route{AllowedFilters: []string{"Status", "Customer.Name", "Status.Name"}}

	for path, want := range map[string]string{
		"/orders/?filter_val_Status=lost":                   "invalid filters",
		"/orders/?order=Status.Name":                        "invalid order",
		"/orders/?order=Unknown":                            "invalid order",
		"/orders/?filter_val_Status=new&filter_op_Status=x": "invalid filters",
	} {
		code, resp := serveJSON(t, h, http.MethodGet, path, 0, nil)
		if code != http.StatusBadRequest || resp.Message == nil || *resp.Message != want {
			t.Fatalf("GET %s returned %d %+v, want 400 with the message %q", path, code, resp, want)
		}
	}
}

func TestServe_ForeignKeyViolation(t *testing.T) {
	h := apiHandler(t)

	code, resp := serveJSON(t, h, http.MethodPut, "/orders/", 1, map[string]any{"customer_id": 99, "status": "new"})
	if code != http.StatusConflict || resp.Code != jsonresp.CodeForeignKeyViolation {
		t.Fatalf("creating an order of a customer that does not exist returned %d %+v, want %d %s", code, resp, http.StatusConflict, jsonresp.CodeForeignKeyViolation)
	}

	serveJSON(t, h, http.MethodPut, "/customers/", 1, map[string]any{"name": "Acme"})
	if code, resp := serveJSON(t, h, http.MethodPut, "/orders/", 1, map[string]any{"customer_id": 1, "status": "new"}); code != http.StatusCreated {
		t.Fatalf("creating an order returned %d %+v", code, resp)
	}

	code, resp = serveJSON(t, h, http.MethodDelete, "/customers/1", 1, nil)
	if code != http.StatusConflict || resp.Code != jsonresp.CodeForeignKeyViolation {
		t.Fatalf("deleting a customer with orders returned %d %+v, want %d %s", code, resp, http.StatusConflict, jsonresp.CodeForeignKeyViolation)
	}
}

func TestServe_UniqueViolation(t *testing.T) {
	recreateTestStructTable()

	obj := fixtures.TestStructWithData()
	for i, want := range []int{http.StatusCreated, http.StatusConflict} {
		code, resp := serveJSON(t, testHandler, http.MethodPut, "/teststruct/", 1, obj)
		if code != want || (i == 1 && resp.Code != jsonresp.CodeUniqueViolation) {
			t.Fatalf("request %d returned %d %+v, want %d", i+1, code, resp, want)
		}
	}
}

func TestServe_ModificationFields(t *testing.T) {
	h := apiHandler(t)
	ctx := context.Background()
	forgedAt := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	before := time.Now().Add(-time.Second)
	code, resp := serveJSON(t, h, http.MethodPut, "/customers/", 7, map[string]any{
		"name": "Acme", "created_at": forgedAt, "created_by": 99,
	})
	if code != http.StatusCreated {
		t.Fatalf("creating a customer returned %d %+v", code, resp)
	}

	customer, err := gocrud.Load[ApiCustomer](ctx, testCRUD, 1)
	if err != nil || customer.CreatedBy != 7 || customer.ModifiedBy != 7 || customer.CreatedAt.Before(before) {
		t.Fatalf("created customer %+v, %v, want it created and modified by 7 now, ignoring the values in the body", customer, err)
	}
	createdAt := customer.CreatedAt

	code, resp = serveJSON(t, h, http.MethodPut, "/customers/1", 8, map[string]any{
		"name": "Acme Inc", "created_at": forgedAt, "created_by": 99,
	})
	if code != http.StatusOK {
		t.Fatalf("updating a customer returned %d %+v", code, resp)
	}

	customer, err = gocrud.Load[ApiCustomer](ctx, testCRUD, 1)
	if err != nil || customer.Name != "Acme Inc" || customer.CreatedBy != 7 || !customer.CreatedAt.Equal(createdAt) || customer.ModifiedBy != 8 {
		t.Fatalf("updated customer %+v, %v, want the name changed by 8, and the creation fields kept", customer, err)
	}
}

func TestServe_ListFilters(t *testing.T) {
	h := apiHandler(t)

	serveJSON(t, h, http.MethodPut, "/customers/", 1, map[string]any{"name": "Acme"})
	shippedAt := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	for _, order := range []map[string]any{
		{"customer_id": 1, "status": "new"},
		{"customer_id": 1, "status": "paid"},
		{"customer_id": 1, "status": "shipped", "shipped_at": shippedAt},
	} {
		if code, resp := serveJSON(t, h, http.MethodPut, "/orders/", 1, order); code != http.StatusCreated {
			t.Fatalf("creating an order returned %d %+v", code, resp)
		}
	}

	tests := map[string][]uint64{
		"/orders/?order=ID&order_direction=asc&filter_val_Status=new,shipped&filter_op_Status=in":                                  {1, 3},
		"/orders/?order=ID&order_direction=asc&filter_val_ShippedAt=&filter_op_ShippedAt=eq":                                       {1, 2},
		"/orders/?order=ID&order_direction=asc&filter_val_ShippedAt=&filter_op_ShippedAt=ne":                                       {3},
		"/orders/?order=ID&order_direction=asc&filter_val_ShippedAt=2024-05-01&filter_op_ShippedAt=ge":                             {3},
		"/orders/?order=ID&order_direction=DESC":                                                                                   {3, 2, 1},
		"/orders/?order=ID&order_direction=asc&filter_val_Status~q=new&filter_val_ShippedAt~q=2024-05-01&filter_op_ShippedAt~q=ge": {1, 3},
		"/orders/?order=ID&order_direction=asc&filter_val_ID~min=2&filter_op_ID~min=ge&filter_val_ID~max=2&filter_op_ID~max=le":    {2},
	}
	for path, want := range tests {
		if got := listIDs(t, h, path); !reflect.DeepEqual(got, want) {
			t.Fatalf("GET %s returned orders %v, want %v", path, got, want)
		}
	}

	code, resp := serveJSON(t, h, http.MethodGet, "/orders/?filter_val_Status~q=new,lost&filter_op_Status~q=in", 0, nil)
	if code != http.StatusBadRequest || resp.Code != jsonresp.CodeValidationFailed {
		t.Fatalf("a list with an invalid value returned %d %+v, want %d %s", code, resp, http.StatusBadRequest, jsonresp.CodeValidationFailed)
	}
	if !reflect.DeepEqual(resp.Data, map[string]any{"Status": "oneof"}) {
		t.Fatalf("a list with an invalid value returned data %v, want the violations {Status: oneof}", resp.Data)
	}
}

func TestServe_EmptyList(t *testing.T) {
	h := apiHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/orders/", nil)
	w := httptest.NewRecorder()
	h.Serve(w, req)

	if want := `{"ok":true,"code":"SUCCESS","data":[]}`; w.Code != http.StatusOK || w.Body.String() != want {
		t.Fatalf("an empty list returned %d %s, want 200 %s", w.Code, w.Body.String(), want)
	}
}

func TestServe_IDInBodyIsIgnored(t *testing.T) {
	h := apiHandler(t)
	ctx := context.Background()

	for _, name := range []string{"Acme", "Globex"} {
		serveJSON(t, h, http.MethodPut, "/customers/", 1, map[string]any{"name": name})
	}

	code, resp := serveJSON(t, h, http.MethodPut, "/customers/1", 1, map[string]any{"id": 2, "name": "Acme Inc"})
	if code != http.StatusOK || resp.Data.(map[string]any)["id"] != float64(1) {
		t.Fatalf("an update with another id in the body returned %d %+v, want the record of the URL updated", code, resp)
	}

	code, resp = serveJSON(t, h, http.MethodPut, "/customers/", 1, map[string]any{"id": 1, "name": "Initech"})
	if code != http.StatusCreated || resp.Data.(map[string]any)["id"] != float64(3) {
		t.Fatalf("a create with an id in the body returned %d %+v, want a new record", code, resp)
	}

	want := map[uint64]string{1: "Acme Inc", 2: "Globex", 3: "Initech"}
	for id, name := range want {
		customer, err := gocrud.Load[ApiCustomer](ctx, testCRUD, id)
		if err != nil || customer.Name != name {
			t.Fatalf("customer %d is %+v, %v, want %s", id, customer, err, name)
		}
	}
}

func TestServe_AllowedFiltersWithGroups(t *testing.T) {
	h := apiHandler(t)
	h.options.Routes["orders"] = Route{AllowedFilters: []string{"Status"}}

	serveJSON(t, h, http.MethodPut, "/customers/", 1, map[string]any{"name": "Acme"})
	for _, status := range []string{"new", "paid"} {
		serveJSON(t, h, http.MethodPut, "/orders/", 1, map[string]any{"customer_id": 1, "status": status})
	}

	path := "/orders/?order=ID&order_direction=asc&filter_val_Status~q=paid&filter_val_ID~q=1"
	if got := listIDs(t, h, path); !reflect.DeepEqual(got, []uint64{2}) {
		t.Fatalf("GET %s returned orders %v, want [2], ignoring the filter that is not allowed", path, got)
	}
}

func TestServe_FilterPaths(t *testing.T) {
	h := apiHandler(t)

	for _, name := range []string{"Acme", "Globex"} {
		serveJSON(t, h, http.MethodPut, "/customers/", 1, map[string]any{"name": name})
	}
	for _, order := range []map[string]any{
		{"customer_id": 1, "status": "new"},
		{"customer_id": 2, "status": "paid"},
		{"customer_id": 1, "status": "paid"},
	} {
		serveJSON(t, h, http.MethodPut, "/orders/", 1, order)
	}

	byPath := "/orders/?order=ID&order_direction=asc&filter_val_Customer.Name=Acme"
	if got := listIDs(t, h, byPath); !reflect.DeepEqual(got, []uint64{1, 2, 3}) {
		t.Fatalf("GET %s returned orders %v, want all, as a path is not allowed unless it is listed", byPath, got)
	}
	sorted := "/orders/?order=Customer.Name&order_direction=desc"
	if code, resp := serveJSON(t, h, http.MethodGet, sorted, 0, nil); code != http.StatusBadRequest || !reflect.DeepEqual(resp.Data, map[string]any{"Customer.Name": "type"}) {
		t.Fatalf("GET %s returned %d %+v, want 400, as the path is not allowed unless it is listed", sorted, code, resp)
	}

	h.options.Routes["orders"] = Route{AllowedFilters: []string{"Status", "Customer.Name"}}

	tests := map[string][]uint64{
		byPath: {1, 3},
		"/orders/?order=ID&order_direction=asc&filter_val_Customer.Name~q=Globex&filter_val_Status~q=new": {1, 2},
		"/orders/?order=Customer.Name&order_direction=asc&filter_val_Status=paid":                         {3, 2},
		"/orders/?order=ID&order_direction=asc&filter_val_Customer.ID=2":                                  {1, 2, 3},
	}
	for path, want := range tests {
		if got := listIDs(t, h, path); !reflect.DeepEqual(got, want) {
			t.Fatalf("GET %s returned orders %v, want %v", path, got, want)
		}
	}
}

func TestServe_InvalidOrder(t *testing.T) {
	h := apiHandler(t)

	code, resp := serveJSON(t, h, http.MethodGet, "/orders/?order=Unknown&order_direction=asc", 0, nil)
	if code != http.StatusBadRequest || resp.Code != jsonresp.CodeValidationFailed || !reflect.DeepEqual(resp.Data, map[string]any{"Unknown": "type"}) {
		t.Fatalf("a list ordered by an unknown field returned %d %+v, want 400 with a type violation", code, resp)
	}
}

type ApiArticle struct {
	ID      uint64 `json:"id"`
	Title   string `json:"title"`
	Version int64  `json:"version" crud:"version"`
}

func TestServe_VersionConflict(t *testing.T) {
	ctx := context.Background()
	_ = testCRUD.DropTable(ctx, &ApiArticle{})
	reg := registry.New(map[string]func() any{
		"articles": func() any { return &ApiArticle{} },
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
	if err := reg.CreateTables(ctx); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}
	h := New(reg, Options{Routes: map[string]Route{"articles": {}}})

	if code, resp := serveJSON(t, h, http.MethodPut, "/articles/", 1, map[string]any{"title": "a"}); code != http.StatusCreated {
		t.Fatalf("creating an article returned %d %+v", code, resp)
	}
	for i, test := range []struct {
		body     map[string]any
		wantCode int
	}{
		{map[string]any{"title": "b", "version": 1}, http.StatusOK},
		{map[string]any{"title": "c", "version": 1}, http.StatusConflict},
		// without a version, the update has the version that was read
		{map[string]any{"title": "d"}, http.StatusOK},
	} {
		code, resp := serveJSON(t, h, http.MethodPut, "/articles/1", 1, test.body)
		if code != test.wantCode || code == http.StatusConflict && resp.Code != jsonresp.CodeVersionConflict {
			t.Fatalf("update %d returned %d %+v, want %d", i+1, code, resp, test.wantCode)
		}
	}
	article, err := gocrud.Load[ApiArticle](ctx, testCRUD, 1)
	if err != nil || article.Title != "d" || article.Version != 3 {
		t.Fatalf("Load returned %+v, %v, want d with version 3", article, err)
	}
}
