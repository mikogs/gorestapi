package gorestapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

func serveProblem(t *testing.T, h http.Handler, method string, path string, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if contentType := w.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Fatalf("%s %s returned the content type %q, want application/problem+json", method, path, contentType)
	}
	var problem map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		t.Fatalf("%s %s returned %q, which is not JSON: %v", method, path, w.Body.String(), err)
	}
	return w, problem
}

func TestProblems(t *testing.T) {
	recreateTestStructTable()
	handler := func(options Options) http.Handler {
		options.Routes = map[string]Route{"teststruct": {}, "readonly": {RegistryKey: "teststruct", Flags: DisableCreate | DisableUpdate | DisableDelete}}
		h := New(testService, options)
		// the instance is the path that the client sent, before the prefix is stripped
		mux := http.NewServeMux()
		mux.Handle("/api/", http.StripPrefix("/api", http.HandlerFunc(h.Serve)))
		return mux
	}

	_, problem := serveProblem(t, handler(Options{}), http.MethodPut, "/api/teststruct/", `{"Age": 500}`)
	want := map[string]any{
		"type":     "about:blank",
		"title":    "Bad Request",
		"status":   float64(400),
		"detail":   "The record has invalid fields, see errors.",
		"instance": "/api/teststruct/",
		"code":     jsonresp.CodeValidationFailed,
	}
	errors, _ := problem["errors"].(map[string]any)
	delete(problem, "errors")
	if !reflect.DeepEqual(problem, want) || errors["Age"] != "max" {
		t.Fatalf("an invalid record returned %v with errors %v, want %v with the max violation of Age", problem, errors, want)
	}

	withTypes := handler(Options{ProblemTypeBaseURL: "https://api.example.com/problems/"})
	_, problem = serveProblem(t, withTypes, http.MethodGet, "/api/teststruct/999", "")
	want = map[string]any{
		"type":     "https://api.example.com/problems/not-found",
		"title":    "Not found",
		"status":   float64(404),
		"detail":   "There is no record with the ID.",
		"instance": "/api/teststruct/999",
		"code":     jsonresp.CodeNotFound,
	}
	if !reflect.DeepEqual(problem, want) {
		t.Fatalf("a missing record returned %v, want %v", problem, want)
	}

	_, problem = serveProblem(t, withTypes, http.MethodGet, "/api/unknown/", "")
	if problem["code"] != jsonresp.CodeNotFound || problem["detail"] != "There is no route for the URL." {
		t.Fatalf("an unknown route returned %v, want the problem of a missing route", problem)
	}

	for path, allow := range map[string]string{
		"/api/readonly/":  "OPTIONS, GET",
		"/api/readonly/1": "OPTIONS, GET",
	} {
		for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
			w, problem := serveProblem(t, withTypes, method, path, "{}")
			if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != allow || problem["code"] != CodeNotAllowed ||
				problem["type"] != "https://api.example.com/problems/not-allowed" {
				t.Fatalf("%s %s returned %d with Allow %q and %v, want 405 with Allow %q", method, path, w.Code, w.Header().Get("Allow"), problem, allow)
			}
		}
	}
}

func TestProblemTypes(t *testing.T) {
	for code, problemType := range problemTypes {
		if problemType.status < 400 || problemType.title == "" || problemType.detail == "" || http.StatusText(problemType.status) == "" {
			t.Fatalf("the problem type of %s is %+v, want an error status, a title and a detail", code, problemType)
		}
		if slug := problemSlug(code); strings.ContainsAny(slug, "_ ") || slug != strings.ToLower(slug) {
			t.Fatalf("the slug of %s is %q, want lower case letters and hyphens", code, slug)
		}
	}
}

func TestBodies(t *testing.T) {
	recreateTestStructTable()
	h := New(testService, Options{Routes: map[string]Route{"teststruct": {}}, MaxBodyBytes: 200})
	for name, test := range map[string]struct {
		contentType string
		body        string
		status      int
		code        string
		errors      map[string]any
	}{
		"not JSON":         {"text/plain", `{}`, http.StatusUnsupportedMediaType, jsonresp.CodeUnsupportedMediaType, nil},
		"no content type":  {"", `{}`, http.StatusUnsupportedMediaType, jsonresp.CodeUnsupportedMediaType, nil},
		"too large":        {"application/json", `{"FirstName":"` + strings.Repeat("a", 300) + `"}`, http.StatusRequestEntityTooLarge, jsonresp.CodeRequestTooLarge, nil},
		"an unknown field": {"application/json", `{"FirstName":"Jo","Nickname":"J"}`, http.StatusBadRequest, jsonresp.CodeValidationFailed, map[string]any{"Nickname": "unknown"}},
		"invalid JSON":     {"application/json", `{"FirstName":`, http.StatusBadRequest, jsonresp.CodeUnmarshalRequest, nil},
	} {
		req := httptest.NewRequest(http.MethodPut, "/teststruct/", strings.NewReader(test.body))
		if test.contentType != "" {
			req.Header.Set("Content-Type", test.contentType)
		}
		w := httptest.NewRecorder()
		h.Serve(w, req)
		var problem map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &problem)
		errors, _ := problem["errors"].(map[string]any)
		if w.Code != test.status || problem["code"] != test.code || test.errors != nil && !reflect.DeepEqual(errors, test.errors) {
			t.Fatalf("a create with %s returned %d %v, want %d %s with %v", name, w.Code, problem, test.status, test.code, test.errors)
		}
	}

	// the fields of a newer version of a client, with AllowUnknownFields
	h = New(testService, Options{Routes: map[string]Route{"teststruct": {}}, AllowUnknownFields: true})
	body, _ := json.Marshal(map[string]any{"Nickname": "J"})
	req := httptest.NewRequest(http.MethodPut, "/teststruct/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Serve(w, req)
	if strings.Contains(w.Body.String(), "unknown") {
		t.Fatalf("a create with an unknown field and AllowUnknownFields returned %d %s", w.Code, w.Body.String())
	}
}
