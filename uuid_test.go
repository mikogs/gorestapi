package gorestapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

type ApiTicket struct {
	ID         string    `json:"id"`
	Title      string    `json:"title" validate:"required"`
	Owner      string    `json:"owner"`
	CreatedAt  time.Time `json:"created_at"`
	CreatedBy  string    `json:"created_by"`
	ModifiedAt time.Time `json:"modified_at"`
	ModifiedBy string    `json:"modified_by"`
}

func ticketsHandler(t *testing.T, route Route) *Handler {
	t.Helper()
	ctx := context.Background()

	_ = testCRUD.DropTable(ctx, &ApiTicket{})
	reg := registry.New(map[string]func() any{
		"tickets": func() any { return &ApiTicket{} },
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
	if err := reg.CreateTables(ctx); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}

	return New(reg, Options{
		Routes: map[string]Route{"tickets": route},
		UserIDFunc: func(r *http.Request) any {
			return r.Header.Get("X-User")
		},
	})
}

func serveTicket(t *testing.T, h *Handler, method string, path string, body any) (int, jsonresp.Response) {
	t.Helper()

	data, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("X-User", "auth0|42")
	w := httptest.NewRecorder()
	h.Serve(w, req)

	var resp jsonresp.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response %q: %v", w.Body.String(), err)
	}
	return w.Code, resp
}

func createTicket(t *testing.T, h *Handler, title string) string {
	t.Helper()

	code, resp := serveTicket(t, h, http.MethodPut, "/tickets/", map[string]any{"title": title})
	if code != http.StatusCreated {
		t.Fatalf("creating a ticket returned %d %+v", code, resp)
	}

	id, _ := resp.Data.(map[string]any)["id"].(string)
	if parsed, err := uuid.Parse(id); err != nil || parsed.String() != id {
		t.Fatalf("creating a ticket returned id %#v, want a UUID", resp.Data)
	}
	return id
}

func TestServe_UUID(t *testing.T) {
	h := ticketsHandler(t, Route{})
	ctx := context.Background()

	id := createTicket(t, h, "broken")

	for _, path := range []string{"/tickets/" + id, "/tickets/" + strings.ToUpper(id)} {
		code, resp := serveTicket(t, h, http.MethodGet, path, nil)
		if code != http.StatusOK || resp.Data.(map[string]any)["created_by"] != "auth0|42" {
			t.Fatalf("GET %s returned %d %+v, want the ticket created by auth0|42", path, code, resp)
		}
	}

	for _, path := range []string{"/tickets/" + uuid.NewString(), "/tickets/42"} {
		if code, resp := serveTicket(t, h, http.MethodGet, path, nil); code != http.StatusNotFound {
			t.Fatalf("GET %s returned %d %+v, want 404", path, code, resp)
		}
	}

	code, resp := serveTicket(t, h, http.MethodPut, "/tickets/"+strings.ToUpper(id), map[string]any{"id": uuid.NewString(), "title": "fixed"})
	if code != http.StatusOK || resp.Data.(map[string]any)["id"] != id {
		t.Fatalf("an update returned %d %+v, want the ticket of the URL updated", code, resp)
	}
	if ticket, err := gocrud.Load[ApiTicket](ctx, testCRUD, id); err != nil || ticket.Title != "fixed" {
		t.Fatalf("the ticket is %+v, %v, want it fixed", ticket, err)
	}

	code, resp = serveTicket(t, h, http.MethodPut, "/tickets/", map[string]any{"id": id, "title": "copy"})
	if newID := resp.Data.(map[string]any)["id"]; code != http.StatusCreated || newID == id {
		t.Fatalf("a create with an id in the body returned %d %+v, want a new ticket", code, resp)
	}

	for _, want := range []int{http.StatusOK, http.StatusNotFound} {
		if code, resp := serveTicket(t, h, http.MethodDelete, "/tickets/"+id, nil); code != want {
			t.Fatalf("DELETE returned %d %+v, want %d", code, resp, want)
		}
	}
	if code, resp := serveTicket(t, h, http.MethodDelete, "/tickets/42", nil); code != http.StatusNotFound {
		t.Fatalf("DELETE of an invalid id returned %d %+v, want 404", code, resp)
	}
}

func TestServe_UUIDList(t *testing.T) {
	h := ticketsHandler(t, Route{})

	ids := []string{createTicket(t, h, "a"), createTicket(t, h, "b"), createTicket(t, h, "c")}

	path := "/tickets/?order=ID&order_direction=asc&filter_op_ID=in&filter_val_ID=" + strings.ToUpper(ids[0]) + "," + ids[2]
	code, resp := serveTicket(t, h, http.MethodGet, path, nil)
	if code != http.StatusOK || len(resp.Data.([]any)) != 2 {
		t.Fatalf("GET %s returned %d %+v, want 2 tickets", path, code, resp)
	}

	code, resp = serveTicket(t, h, http.MethodGet, "/tickets/?filter_val_ID=42", nil)
	if code != http.StatusBadRequest || resp.Data.(map[string]any)["ID"] != gocrud.ViolationType {
		t.Fatalf("a list with an invalid id returned %d %+v, want 400 with a type violation", code, resp)
	}
}

func TestServe_UUIDFilterRead(t *testing.T) {
	h := ticketsHandler(t, Route{FilterRead: func(r *http.Request) FilterSet {
		return FilterSet{Vals: map[string]string{"Owner": r.Header.Get("X-Owner")}}
	}})

	code, resp := serveTicket(t, h, http.MethodPut, "/tickets/", map[string]any{"title": "a", "owner": "alice"})
	if code != http.StatusCreated {
		t.Fatalf("creating a ticket returned %d %+v", code, resp)
	}
	id := resp.Data.(map[string]any)["id"].(string)

	for _, path := range []string{"/tickets/" + id, "/tickets/42"} {
		if code, resp := serveTicket(t, h, http.MethodGet, path, nil); code != http.StatusNotFound {
			t.Fatalf("GET %s of a ticket of another owner returned %d %+v, want 404", path, code, resp)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/tickets/"+strings.ToUpper(id), nil)
	req.Header.Set("X-Owner", "alice")
	w := httptest.NewRecorder()
	h.Serve(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET of a ticket of its owner returned %d %s, want 200", w.Code, w.Body.String())
	}
}
