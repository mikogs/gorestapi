package gorestapi

import (
	"context"
	"errors"
	"net/http"

	svccrud "github.com/mikogs/gocrud/pkg/service"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

func (h *Handler) handleAPIRead(ctx context.Context, w http.ResponseWriter, r *http.Request, key string, route Route, id string) {
	var obj interface{}

	if route.FilterRead != nil {
		scope := route.FilterRead(r)
		filters := svccrud.Filters{
			Vals:      map[string]string{"ID": id},
			ScopeVals: scope.Vals,
			ScopeOps:  scope.Ops,
		}

		objs, _, listErr := h.svc.List(ctx, key, 1, 0, "", "", filters, nil, route.ReadConstructor, nil)
		var validErr *svccrud.FilterValidationError
		if errors.As(listErr, &validErr) && validErr.Violations["ID"] != "" {
			objs, listErr = nil, nil
		}
		if listErr != nil {
			jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
				Ok:   true,
				Code: CodeServiceError,
			})
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
		obj, _, err = h.svc.Read(ctx, key, id, route.ReadConstructor, nil)
		if err != nil {
			if isNotFound(err) {
				jsonresp.Write(w, http.StatusNotFound, &jsonresp.Response{
					Ok:   true,
					Code: jsonresp.CodeNotFound,
				})
				return
			}
			jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
				Ok:   true,
				Code: CodeServiceError,
			})
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
			jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
				Ok:   true,
				Code: CodeServiceError,
			})
			return
		}
	}

	jsonresp.Write(w, http.StatusOK, &jsonresp.Response{
		Ok:   true,
		Code: jsonresp.CodeSuccess,
		Data: responseData(obj, h.svc.PasswordFieldNames(key)),
	})
}
