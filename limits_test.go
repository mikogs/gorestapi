package gorestapi

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
)

type LimitStruct struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// LimitStruct_Slow is LimitStruct with a select query that takes a second.
type LimitStruct_Slow struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

func (s *LimitStruct_Slow) BuildSelectPrefixQuery(tableNamePrefix string) (string, error) {
	return `SELECT "id","name" FROM "` + tableNamePrefix + `limit_struct" CROSS JOIN pg_sleep(1)`, nil
}

func limitHandler(t *testing.T, route Route, options Options) *Handler {
	t.Helper()
	ctx := context.Background()

	_ = testCRUD.DropTable(ctx, &LimitStruct{})
	if err := testCRUD.CreateTable(ctx, &LimitStruct{}); err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}
	if err := testCRUD.Save(ctx, &LimitStruct{Name: "a"}, gocrud.SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	reg := registry.New(map[string]func() any{"limits": func() any { return &LimitStruct{} }}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
	options.Routes = map[string]Route{"limits": route}
	return New(reg, options)
}

// groupFilters returns n filters on Name, each in a group of its own, which are joined with AND.
func groupFilters(n int) string {
	filters := make([]string, n)
	for i := range filters {
		filters[i] = fmt.Sprintf("filter_val_Name~g%d=a", i)
	}
	return strings.Join(filters, "&")
}

func inValues(n int) string {
	values := make([]string, n)
	for i := range values {
		values[i] = fmt.Sprint(i + 1)
	}
	return "filter_op_ID=in&filter_val_ID=" + strings.Join(values, ",")
}

func fatalIfNotBadRequest(t *testing.T, h *Handler, path string, want map[string]string) {
	t.Helper()

	code, resp := serveJSON(t, h, http.MethodGet, path, 0, nil)
	if code != http.StatusBadRequest || !reflect.DeepEqual(resp.Errors, want) {
		t.Fatalf("GET %s returned %d %+v, want 400 with %v", path, code, resp, want)
	}
}

func TestAPI_List_MatchFilter(t *testing.T) {
	fatalIfNotBadRequest(t, limitHandler(t, Route{}, Options{}), "/limits/?filter_op_Name=match&filter_val_Name=^a", map[string]string{"Name": "type"})

	h := limitHandler(t, Route{Flags: EnableMatchFilters}, Options{})
	if got := listIDs(t, h, "/limits/?filter_op_Name=match&filter_val_Name=^a"); !reflect.DeepEqual(got, []uint64{1}) {
		t.Fatalf("a match filter on a route that enables it returned %v, want [1]", got)
	}
}

func TestAPI_List_MaxFilters(t *testing.T) {
	h := limitHandler(t, Route{}, Options{})
	fatalIfNotBadRequest(t, h, "/limits/?"+groupFilters(DefaultMaxFilters+1), map[string]string{"filters": "max"})
	if got := listIDs(t, h, "/limits/?"+groupFilters(DefaultMaxFilters)); !reflect.DeepEqual(got, []uint64{1}) {
		t.Fatalf("%d filters returned %v, want [1]", DefaultMaxFilters, got)
	}

	h = limitHandler(t, Route{}, Options{MaxFilters: 2})
	fatalIfNotBadRequest(t, h, "/limits/?"+groupFilters(3), map[string]string{"filters": "max"})

	h = limitHandler(t, Route{}, Options{MaxFilters: -1})
	if got := listIDs(t, h, "/limits/?"+groupFilters(DefaultMaxFilters+5)); !reflect.DeepEqual(got, []uint64{1}) {
		t.Fatalf("filters without a limit returned %v, want [1]", got)
	}
}

func TestAPI_List_MaxInValues(t *testing.T) {
	h := limitHandler(t, Route{}, Options{})
	fatalIfNotBadRequest(t, h, "/limits/?"+inValues(DefaultMaxInValues+1), map[string]string{"ID": "max"})
	if got := listIDs(t, h, "/limits/?"+inValues(DefaultMaxInValues)); !reflect.DeepEqual(got, []uint64{1}) {
		t.Fatalf("%d values of in returned %v, want [1]", DefaultMaxInValues, got)
	}

	h = limitHandler(t, Route{}, Options{MaxInValues: 3})
	fatalIfNotBadRequest(t, h, "/limits/?"+inValues(4), map[string]string{"ID": "max"})

	h = limitHandler(t, Route{}, Options{MaxInValues: -1})
	if got := listIDs(t, h, "/limits/?"+inValues(DefaultMaxInValues+50)); !reflect.DeepEqual(got, []uint64{1}) {
		t.Fatalf("values of in without a limit returned %v, want [1]", got)
	}
}

func TestAPI_Timeout(t *testing.T) {
	slow := Route{ListConstructor: func() any { return &LimitStruct_Slow{} }}

	h := limitHandler(t, slow, Options{Timeout: 100 * time.Millisecond})
	start := time.Now()
	code, resp := serveJSON(t, h, http.MethodGet, "/limits/", 0, nil)
	if elapsed := time.Since(start); code != http.StatusInternalServerError || elapsed > 700*time.Millisecond {
		t.Fatalf("a query longer than the timeout returned %d %+v after %v, want 500 after the timeout", code, resp, elapsed)
	}
	if running := runningSleeps(t, 300*time.Millisecond); running != 0 {
		t.Fatalf("%d queries are still running in the database after the timeout, want them cancelled", running)
	}

	h = limitHandler(t, slow, Options{Timeout: -1})
	if got := listIDs(t, h, "/limits/"); !reflect.DeepEqual(got, []uint64{1}) {
		t.Fatalf("a query without a timeout returned %v, want [1]", got)
	}
}

// runningSleeps returns the number of the queries with pg_sleep that are running in the database, after waiting up
// to wait for them to be cancelled.
func runningSleeps(t *testing.T, wait time.Duration) int {
	t.Helper()

	var running int
	for deadline := time.Now().Add(wait); ; time.Sleep(20 * time.Millisecond) {
		err := testDB.QueryRow(`SELECT COUNT(*) FROM pg_stat_activity WHERE state = 'active' AND query LIKE '%pg_sleep%' AND pid <> pg_backend_pid()`).Scan(&running)
		if err != nil {
			t.Fatalf("pg_stat_activity failed: %v", err)
		}
		if running == 0 || time.Now().After(deadline) {
			return running
		}
	}
}
