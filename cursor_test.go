package gorestapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

type ApiPost struct {
	ID    uint64 `json:"id"`
	Score int64  `json:"score"`
	Note  *string
}

func postsHandler(t *testing.T, options Options) *Handler {
	t.Helper()
	ctx := context.Background()
	_ = testCRUD.DropTable(ctx, &ApiPost{})
	reg := registry.New(map[string]func() any{
		"posts": func() any { return &ApiPost{} },
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
	if err := reg.CreateTables(ctx); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}
	for i := 0; i < 7; i++ {
		if err := testCRUD.Save(ctx, &ApiPost{Score: int64(i % 3)}, gocrud.SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}
	if options.Routes == nil {
		options.Routes = map[string]Route{"posts": {}}
	}
	return New(reg, options)
}

func TestServe_ListAfter(t *testing.T) {
	h := postsHandler(t, Options{})

	all := listIDs(t, h, "/posts/?limit=100&order=Score&order_direction=desc")
	var got []uint64
	after := ""
	for page := 0; page < 10; page++ {
		code, resp := serveJSON(t, h, http.MethodGet, "/posts/?limit=3&order=Score&order_direction=desc&after="+url.QueryEscape(after), 0, nil)
		if code != http.StatusOK {
			t.Fatalf("the page %d returned %d %+v", page, code, resp)
		}
		data := resp.Data.([]any)
		for _, obj := range data {
			got = append(got, uint64(obj.(map[string]any)["id"].(float64)))
		}
		if len(data) < 3 {
			w := httptest.NewRecorder()
			h.Serve(w, httptest.NewRequest(http.MethodGet, "/posts/?limit=3&order=Score&order_direction=desc&after="+url.QueryEscape(after), nil))
			if strings.Contains(w.Body.String(), `"meta"`) {
				t.Fatalf("the last page returned %s, want no meta", w.Body.String())
			}
			break
		}
		after = resp.Meta.(map[string]any)["next"].(string)
	}
	if !reflect.DeepEqual(got, all) {
		t.Fatalf("the pages returned %v, want %v", got, all)
	}

	for name, path := range map[string]string{
		"another order": "/posts/?limit=3&order=Score&order_direction=asc&after=" + url.QueryEscape(after),
		"not a cursor":  "/posts/?limit=3&after=x",
		"with offset":   "/posts/?limit=3&offset=3&order=Score&order_direction=desc&after=" + url.QueryEscape(after),
	} {
		code, resp := serveJSON(t, h, http.MethodGet, path, 0, nil)
		if code != http.StatusBadRequest || resp.Code != jsonresp.CodeValidationFailed || *resp.Message != "invalid cursor" {
			t.Fatalf("a list with %s returned %d %+v, want 400 invalid cursor", name, code, resp)
		}
	}

	// a nullable field cannot be in a cursor, so the pages of its order are read with offset
	if code, resp := serveJSON(t, h, http.MethodGet, "/posts/?limit=3&order=Note", 0, nil); code != http.StatusOK || resp.Meta != nil {
		t.Fatalf("a list ordered by a nullable field returned %d %+v, want no cursor", code, resp)
	}
}

// recordingHandler records the levels and messages of the logs.
type recordingHandler struct {
	mu      sync.Mutex
	records []string
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Level.String()+" "+record.Message)
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func TestServe_Logger(t *testing.T) {
	records := &recordingHandler{}
	failing := errors.New("failing hook")
	h := postsHandler(t, Options{
		Logger: slog.New(records),
		Routes: map[string]Route{"posts": {
			PostListItem: func(any, *http.Request) error { return failing },
			PreCreate:    func(any, *http.Request) error { return failing },
		}},
	})

	if code, _ := serveJSON(t, h, http.MethodGet, "/posts/", 0, nil); code != http.StatusInternalServerError {
		t.Fatalf("a list with a failing PostListItem returned %d, want 500", code)
	}
	if code, _ := serveJSON(t, h, http.MethodPut, "/posts/", 0, map[string]any{"score": 1}); code != http.StatusInternalServerError {
		t.Fatalf("a create with a failing PreCreate returned %d, want 500", code)
	}
	if code, _ := serveJSON(t, h, http.MethodGet, "/posts/a.b", 0, nil); code != http.StatusBadRequest {
		t.Fatalf("a read of an invalid ID returned %d, want 400", code)
	}
	want := []string{"ERROR error of PostListItem", "ERROR error of PreCreate", "DEBUG invalid ID in the URL"}
	if !reflect.DeepEqual(records.records, want) {
		t.Fatalf("the handler logged %v, want %v", records.records, want)
	}
}

func TestRoutePath(t *testing.T) {
	h := New(nil, Options{Routes: map[string]Route{"posts": {}, "users": {}}})
	for path, want := range map[string]string{"/posts/": "posts", "/posts/42": "posts", "/users/a/b": "users", "/other/1": "", "/posts": ""} {
		if got := h.RoutePath(httptest.NewRequest(http.MethodGet, path, nil)); got != want {
			t.Fatalf("RoutePath of %s returned %q, want %q", path, got, want)
		}
	}
}
