package gorestapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

func (h *Handler) handleAPIRead(ctx context.Context, w http.ResponseWriter, r *http.Request, key string, route Route, id string) {
	var obj any

	if route.FilterRead != nil {
		scope := route.FilterRead(r)
		filters := registry.Filters{
			Vals:      map[string]string{"ID": id},
			ScopeVals: scope.Vals,
			ScopeOps:  scope.Ops,
		}

		objs, listErr := h.reg.List(ctx, key, registry.ListOptions{Limit: 1, Filters: filters, Constructor: route.ReadConstructor})
		var validErr *registry.FilterValidationError
		if errors.As(listErr, &validErr) && validErr.Violations["ID"] != "" {
			objs, listErr = nil, nil
		}
		if listErr != nil {
			writeServiceError(w)
			return
		}
		if len(objs) == 0 {
			jsonresp.Write(w, http.StatusNotFound, &jsonresp.Response{
				Ok:   true,
				Code: jsonresp.CodeNotFound,
			})
			return
		}
		obj = objs[0]
	} else {
		var err error
		obj, err = h.reg.Read(ctx, key, id, route.ReadConstructor)
		if err != nil {
			if isNotFound(err) {
				jsonresp.Write(w, http.StatusNotFound, &jsonresp.Response{
					Ok:   true,
					Code: jsonresp.CodeNotFound,
				})
				return
			}
			writeServiceError(w)
			return
		}
	}

	if route.AllowRead != nil {
		if err := route.AllowRead(obj, r); err != nil {
			jsonresp.Write(w, http.StatusForbidden, &jsonresp.Response{
				Ok:   true,
				Code: CodeForbidden,
			})
			return
		}
	}

	if route.PostRead != nil {
		if err := route.PostRead(obj, r); err != nil {
			h.logServiceError(ctx, w, "error of PostRead", err)
			return
		}
	}

	jsonresp.Write(w, http.StatusOK, &jsonresp.Response{
		Ok:   true,
		Code: jsonresp.CodeSuccess,
		Data: responseData(obj, h.reg.PasswordFieldNames(key)),
	})
}
