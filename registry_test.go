package gorestapi

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

// shippingRules is a layer of an application in front of the registry, with a rule of its domain: an order cannot be
// shipped without a customer.
type shippingRules struct {
	*registry.CRUD
	saved int
}

func (s *shippingRules) Save(ctx context.Context, obj any, userID any) error {
	if order, ok := obj.(*ApiOrder); ok && order.Status == "shipped" && order.CustomerID == 0 {
		return &registry.ModelValidationError{Err: gocrud.ErrInvalidID, Violations: map[string]string{"CustomerID": "required"}}
	}
	s.saved++
	return s.CRUD.Save(ctx, obj, userID)
}

func TestNew_Registry(t *testing.T) {
	h := apiHandler(t)
	rules := &shippingRules{CRUD: h.reg.(*registry.CRUD)}
	h = New(rules, h.options)

	code, resp := serveJSON(t, h, http.MethodPut, "/orders/", 1, map[string]any{"status": "shipped"})
	if code != http.StatusBadRequest || resp.Code != jsonresp.CodeValidationFailed || !reflect.DeepEqual(resp.Errors, map[string]string{"CustomerID": "required"}) {
		t.Fatalf("a create against the rule returned %d %+v, want the violation of the rule", code, resp)
	}
	if code, resp := serveJSON(t, h, http.MethodPut, "/customers/", 1, map[string]any{"name": "Acme"}); code != http.StatusCreated || rules.saved != 1 {
		t.Fatalf("a create returned %d %+v, with %d saves through the layer, want 201 and 1", code, resp, rules.saved)
	}
}
