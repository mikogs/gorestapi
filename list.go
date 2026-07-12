package gorestapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	svccrud "github.com/mikolajgasior/gocrud/pkg/service"
	"github.com/mikolajgasior/gorestapi/pkg/jsonresp"
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

	var filterVals, filterOps map[string]string

	if route.Flags&DisableFilters == 0 {
		filterVals = make(map[string]string)
		filterOps = make(map[string]string)

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
				field := strings.TrimPrefix(name, filterValPrefix)
				if allowed == nil || allowed[field] {
					filterVals[field] = params.Get(name)
				}
			}
			if filterOpRegexp.MatchString(name) {
				field := strings.TrimPrefix(name, filterOpPrefix)
				if allowed == nil || allowed[field] {
					filterOps[field] = params.Get(name)
				}
			}
		}
	}

	if route.FilterList != nil {
		injected := route.FilterList(r)
		if filterVals == nil {
			filterVals = make(map[string]string)
		}
		if filterOps == nil {
			filterOps = make(map[string]string)
		}
		for k, v := range injected.Vals {
			filterVals[k] = v
		}
		for k, v := range injected.Ops {
			filterOps[k] = v
		}
	}

	objs, err := h.svc.List(ctx, key, limit, offset, order, orderDirection, filterVals, filterOps, nil, route.ListConstructor)
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
