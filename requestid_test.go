package gorestapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
)

func TestRequestID(t *testing.T) {
	for name, test := range map[string]struct {
		header string
		keep   bool
	}{
		"none":           {"", false},
		"from a proxy":   {"a1b2-c3.d4_e5", true},
		"with a newline": {"a\nb", false},
		"too long":       {strings.Repeat("a", 129), false},
	} {
		req := httptest.NewRequest(http.MethodGet, "/teststruct/", nil)
		if test.header != "" {
			req.Header.Set(RequestIDHeader, test.header)
		}
		w := httptest.NewRecorder()
		testHandler.Serve(w, req)
		id := w.Header().Get(RequestIDHeader)
		if !regexpRequestID.MatchString(id) || (id == test.header) != test.keep {
			t.Fatalf("a request with the ID %s returned the ID %q, want it kept: %t", name, id, test.keep)
		}
	}
}

func TestLogHandler(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(LogHandler(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	reg := registry.New(map[string]func() any{"teststruct": func() any { return &ApiCustomer{} }}, testDB,
		gocrud.Options{Dialect: gocrud.DialectPostgres}).WithLogger(logger)
	_ = testCRUD.DropTable(context.Background(), &ApiOrder{})
	_ = testCRUD.DropTable(context.Background(), &ApiCustomer{})
	if err := reg.CreateTables(context.Background()); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}
	logs.Reset()
	h := New(reg, Options{Routes: map[string]Route{"teststruct": {}}, Logger: logger, AccessLog: true})

	req := httptest.NewRequest(http.MethodGet, "/teststruct/999", nil)
	req.Header.Set(RequestIDHeader, "req-1")
	h.Serve(httptest.NewRecorder(), req)

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("the log %q is not JSON: %v", line, err)
		}
		records = append(records, record)
	}
	// the not found of the registry, and the access log of the handler
	if len(records) != 2 || records[0]["reg"] == nil || records[1]["msg"] != "request" {
		t.Fatalf("the request logged %v, want the record of the registry and the access log", records)
	}
	for _, record := range records {
		if record["request_id"] != "req-1" {
			t.Fatalf("the record %v has no request_id req-1", record)
		}
	}
	access := records[1]
	if access["method"] != "GET" || access["route"] != "teststruct" || access["path"] != "/teststruct/999" || access["status"] != float64(404) ||
		access["size"].(float64) <= 0 || access["level"] != "INFO" {
		t.Fatalf("the access log is %v, want the method, route, path, status and size of the request", access)
	}

	// the request ID of a context of no request
	var other bytes.Buffer
	slog.New(LogHandler(slog.NewJSONHandler(&other, nil))).InfoContext(context.Background(), "x")
	if strings.Contains(other.String(), "request_id") {
		t.Fatalf("a log without a request has %s", other.String())
	}
}
