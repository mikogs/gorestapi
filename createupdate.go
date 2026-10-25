package gorestapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi/pkg/jsonreq"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

func (h *Handler) handleAPICreateUpdate(ctx context.Context, w http.ResponseWriter, r *http.Request, key string, route Route, id string) {
	var err error
	var obj any

	if id != "" {
		// Load the existing record, with the override constructor when it is set.
		// The URL id is authoritative; any id in the JSON body is overwritten below.
		obj, err = h.reg.Read(ctx, key, id, route.UpdateConstructor)
		if err != nil {
			if isNotFound(err) {
				h.writeProblem(w, r, jsonresp.CodeNotFound, "", nil)
				return
			}
			h.writeServiceError(w, r)
			return
		}

		if route.AllowUpdate != nil {
			if err := route.AllowUpdate(obj, r); err != nil {
				h.writeProblem(w, r, CodeForbidden, "", nil)
				return
			}
		}
	} else {
		if route.CreateConstructor != nil {
			obj = route.CreateConstructor()
		} else {
			obj = h.reg.New(key)
		}
	}

	err = jsonreq.Unmarshal(r, &obj)
	if err != nil {
		h.writeProblem(w, r, jsonresp.CodeUnmarshalRequest, "", nil)
		return
	}

	// The id in the URL is authoritative, so that an id in the request body cannot make an update or a create
	// write to another record, e.g. one that the Allow hooks would not allow. A create clears the id, and the
	// record gets a new one. The id was read above, so it is valid.
	var urlID any
	if id != "" {
		urlID = id
	}
	if err := gocrud.ObjSetIDValue(obj, urlID); err != nil {
		h.logServiceError(w, r, "error setting the id", err)
		return
	}

	if id == "" && route.AllowCreate != nil {
		if err := route.AllowCreate(obj, r); err != nil {
			h.writeProblem(w, r, CodeForbidden, "", nil)
			return
		}
	}

	if id == "" && route.PreCreate != nil {
		if err := route.PreCreate(obj, r); err != nil {
			h.logServiceError(w, r, "error of PreCreate", err)
			return
		}
	}
	if id != "" && route.PreUpdate != nil {
		if err := route.PreUpdate(obj, r); err != nil {
			h.logServiceError(w, r, "error of PreUpdate", err)
			return
		}
	}

	var userID any
	if h.options.UserIDFunc != nil {
		userID = h.options.UserIDFunc(r)
	}

	err = h.reg.Save(ctx, obj, userID)
	if err != nil {
		var validErr *registry.ModelValidationError
		var uniqErr *gocrud.UniqError
		switch {
		case errors.As(err, &validErr):
			// the error of the registry names the internal fields and relations, and is logged by it
			h.writeProblem(w, r, jsonresp.CodeValidationFailed, "The record has invalid fields, see errors.", validErr.Violations)
		case errors.As(err, &uniqErr):
			h.writeProblem(w, r, jsonresp.CodeUniqueViolation, "", nil)
		case errors.Is(err, gocrud.ErrForeignKeyViolation):
			h.writeProblem(w, r, jsonresp.CodeForeignKeyViolation, "", nil)
		case errors.Is(err, gocrud.ErrVersionConflict):
			h.writeProblem(w, r, jsonresp.CodeVersionConflict, "", nil)
		default:
			h.writeServiceError(w, r)
		}
		return
	}
	status := http.StatusCreated
	code := jsonresp.CodeCreated
	if id != "" {
		status = http.StatusOK
		code = jsonresp.CodeSuccess
	}

	jsonresp.Write(w, status, &jsonresp.Response{
		Ok:   true,
		Code: code,
		Data: map[string]any{
			"id": h.reg.ID(obj),
		},
	})
}
