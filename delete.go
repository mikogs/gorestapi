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
		obj, err := h.reg.Read(ctx, key, id, nil)
		if err != nil {
			if isNotFound(err) {
				h.writeProblem(w, r, jsonresp.CodeNotFound, "", nil)
				return
			}
			h.writeServiceError(w, r)
			return
		}
		if err := route.AllowDelete(obj, r); err != nil {
			h.writeProblem(w, r, CodeForbidden, "", nil)
			return
		}
	}

	err := h.reg.Delete(ctx, key, id)
	if err != nil {
		if isNotFound(err) {
			h.writeProblem(w, r, jsonresp.CodeNotFound, "", nil)
			return
		}

		// other objects reference the object with a foreign key that does not allow deleting it
		if errors.Is(err, gocrud.ErrForeignKeyViolation) {
			h.writeProblem(w, r, jsonresp.CodeForeignKeyViolation, "Other records reference the record.", nil)
			return
		}

		h.writeServiceError(w, r)
		return
	}

	jsonresp.Write(w, http.StatusOK, &jsonresp.Response{
		Ok:   true,
		Code: jsonresp.CodeSuccess,
	})
}
