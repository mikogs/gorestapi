package gorestapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	svccrud "github.com/mikogs/gocrud/pkg/service"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

func (h *Handler) handleAPIList(ctx context.Context, w http.ResponseWriter, r *http.Request, key string, route Route) {
	params := r.URL.Query()

	const maxLimit = 1000

	limit, _ := strconv.Atoi(params.Get("limit"))
	offset, _ := strconv.Atoi(params.Get("offset"))
	if limit < 1 {
		limit = 10
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	order := params.Get("order")
	orderDirection := params.Get("order_direction")

	var filters svccrud.Filters

	if route.Flags&DisableFilters == 0 {
		filters.Vals = make(map[string]string)
		filters.Ops = make(map[string]string)

		// Build allowed-filter lookup once (nil map means all fields are allowed).
		var allowed map[string]bool
		if len(route.AllowedFilters) > 0 {
			allowed = make(map[string]bool, len(route.AllowedFilters))
			for _, f := range route.AllowedFilters {
				allowed[f] = true
			}
		}

		for name := range params {
			if filterValRegexp.MatchString(name) {
				key := strings.TrimPrefix(name, filterValPrefix)
				if isFilterAllowed(allowed, key) {
					filters.Vals[key] = params.Get(name)
				}
			}
			if filterOpRegexp.MatchString(name) {
				key := strings.TrimPrefix(name, filterOpPrefix)
				if isFilterAllowed(allowed, key) {
					filters.Ops[key] = params.Get(name)
				}
			}
		}
	}

	if route.FilterList != nil {
		scope := route.FilterList(r)
		filters.ScopeVals = scope.Vals
		filters.ScopeOps = scope.Ops
	}

	objs, _, err := h.svc.List(ctx, key, limit, offset, order, orderDirection, filters, nil, route.ListConstructor, nil)
	if err != nil {
		var validErr *svccrud.FilterValidationError
		if errors.As(err, &validErr) {
			message := err.Error()
			jsonresp.Write(w, http.StatusBadRequest, &jsonresp.Response{
				Ok:      true,
				Code:    jsonresp.CodeValidationFailed,
				Message: &message,
			})
			return
		}

		jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
			Ok:   true,
			Code: CodeServiceError,
		})
		return
	}

	// an empty list is encoded as [] rather than null
	if objs == nil {
		objs = []interface{}{}
	}

	pwFields := h.svc.PasswordFieldNames(key)
	for i, obj := range objs {
		if route.PostListItem != nil {
			if err := route.PostListItem(obj, r); err != nil {
				jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
					Ok:   true,
					Code: CodeServiceError,
				})
				return
			}
		}
		objs[i] = responseData(obj, pwFields)
	}

	jsonresp.Write(w, http.StatusOK, &jsonresp.Response{
		Ok:   true,
		Code: jsonresp.CodeSuccess,
		Data: objs,
	})
}

// isFilterAllowed checks if the field of a filter key, e.g. Name in Name.q, is allowed. A nil map allows every field.
func isFilterAllowed(allowed map[string]bool, key string) bool {
	field, _, _ := strings.Cut(key, ".")
	return allowed == nil || allowed[field]
}
