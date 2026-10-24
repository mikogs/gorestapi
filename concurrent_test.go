package gorestapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

// request serves a request with the handler, and returns its status and response, or an error, as it is called on
// goroutines, which cannot stop the test.
func request(h *Handler, method string, path string, body any) (int, jsonresp.Response, error) {
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	w := httptest.NewRecorder()
	h.Serve(w, httptest.NewRequest(method, path, bytes.NewReader(data)))

	var resp jsonresp.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		return w.Code, resp, fmt.Errorf("%s %s: %w: %q", method, path, err, w.Body.String())
	}
	return w.Code, resp, nil
}

// TestServe_Concurrent serves the requests of goroutines at the same time with a new handler, whose caches are empty,
// so that the race detector can find the data races of the code that they share.
func TestServe_Concurrent(t *testing.T) {
	h := relationPathsHandler(t, map[string]Route{"customers": {}})

	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			for i := range 10 {
				if err := concurrentRequests(h); err != nil {
					t.Errorf("worker %d, iteration %d: %s", worker, i, err.Error())
					return
				}
			}
		})
	}
	wg.Wait()
}

// concurrentRequests creates, reads, lists, updates and deletes an order.
func concurrentRequests(h *Handler) error {
	code, resp, err := request(h, http.MethodPut, "/orders/", map[string]any{"CustomerID": 1, "Status": "new"})
	if err != nil || code != http.StatusCreated {
		return fmt.Errorf("create returned %d %+v, %w", code, resp, err)
	}
	path := fmt.Sprintf("/orders/%v", resp.Data.(map[string]any)["id"])

	for _, step := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, path, nil},
		{http.MethodGet, "/orders/?filter_val_Customer.Name=Acme&order=Customer.Name&order_direction=desc", nil},
		{http.MethodGet, "/orders/?filter_val_Customer.Email=acme@example.com&filter_val_Status~q=new&filter_val_Customer.Orders.Status~q=paid", nil},
		{http.MethodPut, path, map[string]any{"CustomerID": 1, "Status": "paid"}},
		{http.MethodDelete, path, nil},
	} {
		if code, resp, err := request(h, step.method, step.path, step.body); err != nil || code != http.StatusOK {
			return fmt.Errorf("%s %s returned %d %+v, %w", step.method, step.path, code, resp, err)
		}
	}
	return nil
}
