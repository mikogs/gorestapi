package gorestapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/mikolajgasior/gocrud"
	svccrud "github.com/mikolajgasior/gocrud/pkg/service"
	"github.com/mikolajgasior/gorestapi/pkg/jsonreq"
	"github.com/mikolajgasior/gorestapi/pkg/jsonresp"
)

func (h *Handler) handleAPICreateUpdate(ctx context.Context, w http.ResponseWriter, r *http.Request, key string, route Route, id string) {
	var idInt uint64
	var err error
	var obj interface{}

	if id != "" {
		idInt, err = strconv.ParseUint(id, 10, 64)
		if err != nil {
			jsonresp.Write(w, http.StatusBadRequest, &jsonresp.Response{
				Ok:   true,
				Code: jsonresp.CodeBadRequest,
			})
			return
		}

		// Use the override constructor — skip reading the existing record.
		// The URL id is authoritative; any id in the JSON body is overwritten below.
		obj, _, err = h.svc.Read(ctx, key, idInt, route.UpdateConstructor, nil)
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

	if id == "" && route.AllowCreate != nil {
		if err := route.AllowCreate(obj, r); err != nil {
			jsonresp.Write(w, http.StatusForbidden, &jsonresp.Response{
				Ok:   true,
				Code: CodeForbidden,
			})
			return
		}
	}

	// When an update constructor is used the URL id must win over any id in the
	// JSON body, so set it after unmarshalling.
	if id != "" && route.UpdateConstructor != nil {
		gocrud.ObjSetIDValue(obj, idInt)
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

	now := time.Now().UTC().Unix()
	userID := uint64(0)
	if h.options.UserIDFunc != nil {
		userID = h.options.UserIDFunc(r)
	}

	err = h.svc.Save(ctx, obj, now, userID)
	if err != nil {
		var validErr *svccrud.ModelValidationError
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
	idInt = h.svc.ID(obj)

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
			"id": idInt,
		},
	})
}
