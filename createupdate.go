package gorestapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mikogs/gocrud"
	svccrud "github.com/mikogs/gocrud/pkg/service"
	"github.com/mikogs/gorestapi/pkg/jsonreq"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

func (h *Handler) handleAPICreateUpdate(ctx context.Context, w http.ResponseWriter, r *http.Request, key string, route Route, id string) {
	var err error
	var obj interface{}

	if id != "" {
		// Load the existing record, with the override constructor when it is set.
		// The URL id is authoritative; any id in the JSON body is overwritten below.
		obj, _, err = h.svc.Read(ctx, key, id, route.UpdateConstructor, nil)
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

		if route.AllowUpdate != nil {
			if err := route.AllowUpdate(obj, r); err != nil {
				jsonresp.Write(w, http.StatusForbidden, &jsonresp.Response{
					Ok:   true,
					Code: CodeForbidden,
				})
				return
			}
		}
	} else {
		if route.CreateConstructor != nil {
			obj = route.CreateConstructor()
		} else {
			obj = h.svc.New(key)
		}
	}

	err = jsonreq.Unmarshal(r, &obj)
	if err != nil {
		jsonresp.Write(w, http.StatusBadRequest, &jsonresp.Response{
			Ok:   true,
			Code: jsonresp.CodeUnmarshalRequest,
		})
		return
	}

	// The id in the URL is authoritative, so that an id in the request body cannot make an update or a create
	// write to another record, e.g. one that the Allow hooks would not allow. A create clears the id, and the
	// record gets a new one. The id was read above, so it is valid.
	var urlID interface{}
	if id != "" {
		urlID = id
	}
	if err := gocrud.ObjSetIDValue(obj, urlID); err != nil {
		jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
			Ok:   true,
			Code: CodeServiceError,
		})
		return
	}

	if id == "" && route.AllowCreate != nil {
		if err := route.AllowCreate(obj, r); err != nil {
			jsonresp.Write(w, http.StatusForbidden, &jsonresp.Response{
				Ok:   true,
				Code: CodeForbidden,
			})
			return
		}
	}

	if id == "" && route.PreCreate != nil {
		if err := route.PreCreate(obj, r); err != nil {
			jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
				Ok:   true,
				Code: CodeServiceError,
			})
			return
		}
	}
	if id != "" && route.PreUpdate != nil {
		if err := route.PreUpdate(obj, r); err != nil {
			jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
				Ok:   true,
				Code: CodeServiceError,
			})
			return
		}
	}

	var userID interface{}
	if h.options.UserIDFunc != nil {
		userID = h.options.UserIDFunc(r)
	}

	err = h.svc.Save(ctx, obj, userID)
	if err != nil {
		var validErr *svccrud.ModelValidationError
		var uniqErr *gocrud.UniqError
		switch {
		case errors.As(err, &validErr):
			message := err.Error()
			jsonresp.Write(w, http.StatusBadRequest, &jsonresp.Response{
				Ok:      true,
				Code:    jsonresp.CodeValidationFailed,
				Message: &message,
				Data:    validErr.Violations,
			})
		case errors.As(err, &uniqErr):
			jsonresp.Write(w, http.StatusConflict, &jsonresp.Response{
				Ok:   true,
				Code: jsonresp.CodeUniqueViolation,
			})
		case errors.Is(err, gocrud.ErrForeignKeyViolation):
			jsonresp.Write(w, http.StatusConflict, &jsonresp.Response{
				Ok:   true,
				Code: jsonresp.CodeForeignKeyViolation,
			})
		default:
			jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
				Ok:   true,
				Code: CodeServiceError,
			})
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
		Data: map[string]interface{}{
			"id": h.svc.ID(obj),
		},
	})
}
