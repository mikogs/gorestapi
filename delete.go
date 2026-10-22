package gorestapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

func (h *Handler) handleAPIDelete(ctx context.Context, w http.ResponseWriter, r *http.Request, key string, route Route, id string) {
	if route.AllowDelete != nil {
		obj, _, err := h.svc.Read(ctx, key, id, nil, nil)
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
		if err := route.AllowDelete(obj, r); err != nil {
			jsonresp.Write(w, http.StatusForbidden, &jsonresp.Response{
				Ok:   true,
				Code: CodeForbidden,
			})
			return
		}
	}

	err := h.svc.Delete(ctx, key, id)
	if err != nil {
		if isNotFound(err) {
			jsonresp.Write(w, http.StatusNotFound, &jsonresp.Response{
				Ok:   true,
				Code: jsonresp.CodeNotFound,
			})
			return
		}

		// other objects reference the object with a foreign key that does not allow deleting it
		if errors.Is(err, gocrud.ErrForeignKeyViolation) {
			jsonresp.Write(w, http.StatusConflict, &jsonresp.Response{
				Ok:   true,
				Code: jsonresp.CodeForeignKeyViolation,
			})
			return
		}

		jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
			Ok:   true,
			Code: CodeServiceError,
		})
		return
	}

	jsonresp.Write(w, http.StatusOK, &jsonresp.Response{
		Ok:   true,
		Code: jsonresp.CodeSuccess,
	})
}
