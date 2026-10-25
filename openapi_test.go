package gorestapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
)

func loadOpenAPI(t *testing.T, h *Handler) *openapi3.T {
	t.Helper()
	data, err := h.OpenAPI(OpenAPIInfo{Title: "Test", Version: "1.0.0", Servers: []string{"/"}})
	if err != nil {
		t.Fatalf("OpenAPI failed: %s", err.Error())
	}
	doc, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		t.Fatalf("the document cannot be loaded: %v\n%s", err, data)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("the document is not valid: %v\n%s", err, data)
	}
	return doc
}

// fatalIfNotOfDocument serves a request, and checks that the request and the response are of doc.
func fatalIfNotOfDocument(t *testing.T, doc *openapi3.T, h *Handler, method string, target string, body any, wantStatus int) []byte {
	t.Helper()
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatalf("NewRouter failed: %s", err.Error())
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "1")
	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s is not an operation of the document: %v", method, target, err)
	}
	input := &openapi3filter.RequestValidationInput{Request: req, PathParams: pathParams, Route: route,
		Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}}
	if err := openapi3filter.ValidateRequest(context.Background(), input); err != nil {
		t.Fatalf("%s %s is not a request of the document: %v", method, target, err)
	}
	req.Body = io.NopCloser(bytes.NewReader(data))

	w := httptest.NewRecorder()
	h.Serve(w, req)
	if w.Code != wantStatus {
		t.Fatalf("%s %s returned %d %s, want %d", method, target, w.Code, w.Body.String(), wantStatus)
	}
	responseInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: input, Status: w.Code, Header: w.Header(), Body: io.NopCloser(bytes.NewReader(w.Body.Bytes())),
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	if err := openapi3filter.ValidateResponse(context.Background(), responseInput); err != nil {
		t.Fatalf("the response of %s %s is not of the document: %v\n%s", method, target, err, w.Body.String())
	}
	return w.Body.Bytes()
}

func TestOpenAPI(t *testing.T) {
	h := apiHandler(t)
	h.options.Routes["orders"] = Route{AllowedFilters: []string{"Status", "CustomerID", "Customer.Name"}, Flags: DisableDelete}
	doc := loadOpenAPI(t, h)

	if doc.Paths.Find("/orders/{id}").Delete != nil || doc.Paths.Find("/customers/{id}").Delete == nil {
		t.Fatal("the document has the delete of orders, which the route disables, or not the delete of customers")
	}
	list := doc.Paths.Find("/orders/").Get
	params := map[string]bool{}
	for _, param := range list.Parameters {
		params[param.Value.Name] = true
	}
	for _, name := range []string{"limit", "offset", "after", "order", "filter_val_Status", "filter_op_Customer.Name"} {
		if !params[name] {
			t.Fatalf("the list of orders has the parameters %v, without %s", params, name)
		}
	}
	page := list.Responses.Status(http.StatusOK).Value.Content["application/json"].Schema.Value
	if meta := page.Properties["meta"]; meta == nil || meta.Value.Properties["next"] == nil || page.Properties["ok"].Value.Type.Is("boolean") == false {
		t.Fatalf("the page of orders is %+v, want ok, and meta with next", page.Properties)
	}
	if params["filter_val_ShippedAt"] {
		t.Fatal("the list of orders has a filter of a field that is not in AllowedFilters")
	}
	order := doc.Components.Schemas["ApiOrder"].Value
	if status := order.Properties["status"].Value; len(status.Enum) != 3 {
		t.Fatalf("the status of an order has the enum %v, want the values of its oneof rule", status.Enum)
	}
	if shipped := order.Properties["shipped_at"].Value; !shipped.Nullable || shipped.Format != "date-time" {
		t.Fatalf("the shipped_at of an order is %+v, want a nullable date-time", shipped)
	}
	if customer := order.Properties["customer"].Value; !customer.ReadOnly || len(customer.AllOf) != 1 {
		t.Fatalf("the customer of an order is %+v, want a read-only reference to the customer", customer)
	}
	if customer := doc.Components.Schemas["ApiCustomer"].Value; !customer.Properties["created_at"].Value.ReadOnly ||
		!customer.Properties["id"].Value.ReadOnly || len(customer.Required) != 1 || customer.Required[0] != "name" {
		t.Fatalf("the customer is %+v, want the fields of the server read only, and its name required", customer)
	}

	fatalIfNotOfDocument(t, doc, h, http.MethodPut, "/customers/", map[string]any{"name": "Acme"}, http.StatusCreated)
	fatalIfNotOfDocument(t, doc, h, http.MethodPut, "/orders/", map[string]any{"customer_id": 1, "status": "new"}, http.StatusCreated)
	body := fatalIfNotOfDocument(t, doc, h, http.MethodGet, "/orders/?limit=1&order=Status&filter_val_Status=new", nil, http.StatusOK)
	if !strings.Contains(string(body), `"next"`) {
		t.Fatalf("a full page of orders returned %s, without the cursor of the next page", body)
	}
	fatalIfNotOfDocument(t, doc, h, http.MethodGet, "/orders/1", nil, http.StatusOK)
	fatalIfNotOfDocument(t, doc, h, http.MethodPut, "/orders/1", map[string]any{"customer_id": 1, "status": "paid"}, http.StatusOK)
	fatalIfNotOfDocument(t, doc, h, http.MethodGet, "/orders/999", nil, http.StatusNotFound)
	fatalIfNotOfDocument(t, doc, h, http.MethodPut, "/orders/", map[string]any{"customer_id": 9, "status": "new"}, http.StatusConflict)
	fatalIfNotOfDocument(t, doc, h, http.MethodDelete, "/customers/1", nil, http.StatusConflict)
}

func TestOpenAPI_Passwords(t *testing.T) {
	_, h := passStructService()
	doc := loadOpenAPI(t, h)
	password := doc.Components.Schemas["PassStruct"].Value.Properties["Password"]
	if password == nil || !password.Value.WriteOnly {
		t.Fatalf("the password is %+v, want a write-only property", password)
	}
	for _, param := range doc.Paths.Find("/passstruct/").Get.Parameters {
		if strings.Contains(param.Value.Name, "Password") {
			t.Fatalf("the list has the parameter %s of the password", param.Value.Name)
		}
	}
}

func TestOperationName(t *testing.T) {
	for path, want := range map[string]string{"notes": "Notes", "order-items": "OrderItems", "api_keys": "ApiKeys"} {
		if got := operationName(path); got != want {
			t.Fatalf("operationName(%q) returned %q, want %q", path, got, want)
		}
	}
}
