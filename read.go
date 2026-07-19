package gorestapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	svccrud "github.com/mikolajgasior/gocrud/pkg/service"
	"github.com/mikolajgasior/gorestapi/pkg/jsonresp"
)

func (h *Handler) handleAPIRead(ctx context.Context, w http.ResponseWriter, r *http.Request, key string, route Route, id string) {
	idInt, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		jsonresp.Write(w, http.StatusBadRequest, &jsonresp.Response{
			Ok:   true,
			Code: jsonresp.CodeBadRequest,
		})
		return
	}

	var obj interface{}

	if route.FilterRead != nil {
		injected := route.FilterRead(r)
		filterVals := make(map[string]string)
		filterOps := make(map[string]string)
		for k, v := range injected.Vals {
			filterVals[k] = v
		}
		for k, v := range injected.Ops {
			filterOps[k] = v
		}
		filterVals["ID"] = id // ID constraint always wins

		objs, listErr := h.svc.List(ctx, key, 1, 0, "", "", filterVals, filterOps, nil, route.ReadConstructor)
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
		obj, _, err = h.svc.Read(ctx, key, idInt, route.ReadConstructor, nil)
		if err != nil {
			if errors.Is(err, svccrud.NotFoundError) {
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
