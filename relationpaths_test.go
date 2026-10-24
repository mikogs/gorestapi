package gorestapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
)

type RpCustomer struct {
	ID     uint64    `json:"id"`
	Name   string    `json:"name"`
	Email  string    `json:"-"`
	Orders []RpOrder `json:"orders,omitempty" crud:"rel:CustomerID"`
}

type RpOrder struct {
	ID         uint64      `json:"id"`
	CustomerID uint64      `json:"customer_id"`
	Status     string      `json:"status"`
	Customer   *RpCustomer `json:"customer,omitempty" crud:"rel:CustomerID"`
}

// relationPathsHandler returns a handler with the route orders, which lists paths through Customer, and the routes
// of customers.
func relationPathsHandler(t *testing.T, customerRoutes map[string]Route) *Handler {
	t.Helper()
	ctx := context.Background()

	for _, obj := range []any{&RpOrder{}, &RpCustomer{}} {
		_ = testCRUD.DropTable(ctx, obj)
	}
	for _, obj := range []any{&RpCustomer{}, &RpOrder{}} {
		if err := testCRUD.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed: %v", err)
		}
	}
	for _, obj := range []any{
		&RpCustomer{Name: "Acme", Email: "acme@example.com"}, &RpCustomer{Name: "Globex", Email: "globex@example.com"},
		&RpOrder{CustomerID: 1, Status: "paid"}, &RpOrder{CustomerID: 2, Status: "new"},
	} {
		if err := testCRUD.Save(ctx, obj, gocrud.SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	reg := registry.New(map[string]func() any{
		"customers": func() any { return &RpCustomer{} },
		"orders":    func() any { return &RpOrder{} },
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})

	routes := map[string]Route{
		"orders": {AllowedFilters: []string{"Status", "Customer.ID", "Customer.Name", "Customer.Email", "Customer.Orders.Status"}},
	}
	for path, route := range customerRoutes {
		routes[path] = route
	}
	return New(reg, Options{Routes: routes})
}

func fatalIfOrders(t *testing.T, h *Handler, path string, want []uint64) {
	t.Helper()
	if got := listIDs(t, h, path); !reflect.DeepEqual(got, want) {
		t.Fatalf("GET %s returned orders %v, want %v", path, got, want)
	}
}

func TestAPI_RelationPaths_Allowed(t *testing.T) {
	for name, routes := range map[string]map[string]Route{
		"a route of customers":                {"customers": {}},
		"a route with another registry key":   {"clients": {RegistryKey: "customers"}},
		"a public route next to a scoped one": {"customers": {FilterList: func(*http.Request) FilterSet { return FilterSet{} }}, "public": {RegistryKey: "customers"}},
	} {
		t.Run(name, func(t *testing.T) {
			h := relationPathsHandler(t, routes)
			fatalIfOrders(t, h, "/orders/?order=ID&filter_val_Customer.Name=Acme", []uint64{1})
			fatalIfOrders(t, h, "/orders/?order=Customer.Name&order_direction=desc", []uint64{2, 1})
			fatalIfOrders(t, h, "/orders/?order=ID&filter_val_Customer.Orders.Status=new", []uint64{2})
		})
	}
}

func TestAPI_RelationPaths_Refused(t *testing.T) {
	for name, routes := range map[string]map[string]Route{
		"no route of customers": {},
		"FilterList":            {"customers": {FilterList: func(*http.Request) FilterSet { return FilterSet{} }}},
		"FilterRead":            {"customers": {FilterRead: func(*http.Request) FilterSet { return FilterSet{} }}},
		"AllowRead":             {"customers": {AllowRead: func(any, *http.Request) error { return nil }}},
		"DisableList":           {"customers": {Flags: DisableList}},
		"DisableFilters":        {"customers": {Flags: DisableFilters}},
	} {
		t.Run(name, func(t *testing.T) {
			h := relationPathsHandler(t, routes)
			for _, filter := range []string{"filter_val_Customer.Name=Acme", "filter_val_Customer.ID=1"} {
				fatalIfOrders(t, h, "/orders/?order=ID&"+filter, []uint64{1, 2})
			}
			code, resp := serveJSON(t, h, http.MethodGet, "/orders/?order=Customer.Name", 0, nil)
			if code != http.StatusBadRequest || !reflect.DeepEqual(resp.Data, map[string]any{"Customer.Name": "type"}) {
				t.Fatalf("an order by a path returned %d %+v, want 400", code, resp)
			}
		})
	}
}

func TestAPI_RelationPaths_HiddenField(t *testing.T) {
	h := relationPathsHandler(t, map[string]Route{"customers": {}})
	fatalIfOrders(t, h, "/orders/?order=ID&filter_val_Customer.Email=acme@example.com", []uint64{1, 2})

	h = relationPathsHandler(t, map[string]Route{"customers": {AllowedFilters: []string{"Email"}}})
	fatalIfOrders(t, h, "/orders/?order=ID&filter_val_Customer.Email=acme@example.com", []uint64{1})
}

func TestAPI_RelationPaths_Nested(t *testing.T) {
	h := relationPathsHandler(t, map[string]Route{"customers": {}})
	h.options.Routes["orders"] = Route{
		AllowedFilters: []string{"Customer.Orders.Status"},
		FilterList:     func(*http.Request) FilterSet { return FilterSet{} },
	}
	fatalIfOrders(t, h, "/orders/?order=ID&filter_val_Customer.Orders.Status=new", []uint64{1, 2})
}

func TestAPI_RelationPaths_LoggedOnce(t *testing.T) {
	var logs bytes.Buffer
	defaultLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(defaultLogger)

	h := relationPathsHandler(t, map[string]Route{})
	for range 3 {
		listIDs(t, h, "/orders/?order=ID&filter_val_Customer.Name=Acme")
	}

	if n := strings.Count(logs.String(), `path=Customer.Name reason="no route serves RpCustomer"`); n != 1 {
		t.Fatalf("the refused path was logged %d times, want once:\n%s", n, logs.String())
	}
}
