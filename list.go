package gorestapi

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
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

	responseFields := h.responseFields(key, route)
	allowed := responseFields
	if len(route.AllowedFilters) > 0 {
		allowed = make(map[string]bool, len(route.AllowedFilters))
		for _, f := range route.AllowedFilters {
			if strings.Contains(f, ".") && !h.pathAllowed(ctx, key, f) {
				continue
			}
			allowed[f] = true
		}
	}

	// the objects can be ordered by the fields that they are returned with, and by the allowed filters. An order that
	// is not allowed is a validation error, the same as for a field that does not exist, so that clients cannot find
	// out which fields exist but are not returned.
	order := params.Get("order")
	orderDirection := params.Get("order_direction")
	if order != "" && !isFilterAllowed(allowed, order) && !isFilterAllowed(responseFields, order) {
		h.writeProblem(w, r, jsonresp.CodeValidationFailed, "The order is not a field that can be used, see errors.",
			map[string]string{order: gocrud.ViolationType})
		return
	}

	var filters registry.Filters

	if route.Flags&DisableFilters == 0 {
		filters.Vals = make(map[string]string)
		filters.Ops = make(map[string]string)

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

	if violations := h.checkFilters(route, filters); len(violations) > 0 {
		h.writeProblem(w, r, jsonresp.CodeValidationFailed, "The filters have invalid values, see errors.", violations)
		return
	}

	if route.FilterList != nil {
		scope := route.FilterList(r)
		filters.ScopeVals = scope.Vals
		filters.ScopeOps = scope.Ops
	}

	objs, err := h.reg.List(ctx, key, registry.ListOptions{
		Limit:          limit,
		Offset:         offset,
		Order:          order,
		OrderDirection: orderDirection,
		After:          params.Get("after"),
		Filters:        filters,
		Constructor:    route.ListConstructor,
	})
	if err != nil {
		var validErr *registry.FilterValidationError
		if errors.As(err, &validErr) {
			// the error of the registry names the internal fields and relations, and is logged by it
			message := "The filters have invalid values, see errors."
			switch {
			case errors.Is(err, gocrud.ErrInvalidOrder):
				message = "The order is not a field that can be used, see errors."
			case errors.Is(err, gocrud.ErrCursor):
				message = "The cursor cannot be used for the order, see errors."
			}
			h.writeProblem(w, r, jsonresp.CodeValidationFailed, message, validErr.Violations)
			return
		}

		h.writeServiceError(w, r)
		return
	}

	// an empty list is encoded as [] rather than null
	if objs == nil {
		objs = []any{}
	}

	// a full page is followed by another one, which may be empty. The cursor is left out when the order has fields
	// that cannot be in a cursor, e.g. nullable ones, and the pages are read with offset then.
	var meta *listMeta
	if len(objs) == limit {
		if next, err := h.reg.NextCursor(objs[len(objs)-1], order, orderDirection); err == nil {
			meta = &listMeta{Next: next}
		}
	}

	pwFields := h.reg.PasswordFieldNames(key)
	for i, obj := range objs {
		if route.PostListItem != nil {
			if err := route.PostListItem(obj, r); err != nil {
				h.logServiceError(w, r, "error of PostListItem", err)
				return
			}
		}
		objs[i] = responseData(obj, pwFields)
	}

	response := &jsonresp.Response{
		Ok:   true,
		Code: jsonresp.CodeSuccess,
		Data: objs,
	}
	if meta != nil {
		response.Meta = meta
	}
	jsonresp.Write(w, http.StatusOK, response)
}

// listMeta is the meta of a list response: the cursor of the next page, for the after parameter.
type listMeta struct {
	Next string `json:"next"`
}

// checkFilters returns the violations of the limits of the filters of a client, which keep the queries of a request
// cheap for the database: the number of filters (violation "max" of "filters"), the values of in ("max"), and the
// match operator ("type") unless the route has the flag EnableMatchFilters.
func (h *Handler) checkFilters(route Route, filters registry.Filters) map[string]string {
	violations := map[string]string{}

	if maxFilters := orDefault(h.options.MaxFilters, DefaultMaxFilters); maxFilters > 0 && len(filters.Vals) > maxFilters {
		violations["filters"] = "max"
	}

	maxInValues := orDefault(h.options.MaxInValues, DefaultMaxInValues)
	for key, op := range filters.Ops {
		switch {
		case op == "match" && route.Flags&EnableMatchFilters == 0:
			violations[key] = gocrud.ViolationType
		case op == "in" && maxInValues > 0 && strings.Count(filters.Vals[key], ",")+1 > maxInValues:
			violations[key] = "max"
		}
	}

	return violations
}

// isFilterAllowed checks if the field of a filter key, e.g. Name in Name~q, is allowed.
func isFilterAllowed(allowed map[string]bool, key string) bool {
	field, _, _ := strings.Cut(key, "~")
	return allowed[field]
}

type responseFieldsKey struct {
	key string
	typ reflect.Type
}

// responseFields returns the names of the fields of the objects that List returns for the route, which clients may
// use in filters and in order when the route has no AllowedFilters: the exported fields without the password fields,
// which are removed from the responses, and without the fields with a json:"-" tag. A client cannot query the values
// that it does not get, e.g. with the like operator, which would reveal them. Paths through relation fields query
// other tables, so they are allowed only when they are listed in AllowedFilters.
func (h *Handler) responseFields(key string, route Route) map[string]bool {
	var obj any
	if route.ListConstructor != nil {
		obj = route.ListConstructor()
	} else {
		obj = h.reg.New(key)
	}
	if obj == nil {
		return map[string]bool{}
	}

	cacheKey := responseFieldsKey{key: key, typ: reflect.TypeOf(obj)}
	if fields, ok := h.responseFieldsCache.Load(cacheKey); ok {
		return fields.(map[string]bool)
	}

	passwords := map[string]bool{}
	for _, name := range h.reg.PasswordFieldNames(key) {
		passwords[name] = true
	}

	fields := map[string]bool{}
	t := reflect.Indirect(reflect.ValueOf(obj)).Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.IsExported() && !passwords[field.Name] && field.Tag.Get("json") != "-" {
			fields[field.Name] = true
		}
	}

	h.responseFieldsCache.Store(cacheKey, fields)
	return fields
}
