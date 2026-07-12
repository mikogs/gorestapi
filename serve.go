package gorestapi

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/mikolajgasior/gocrud/pkg/logger"
	"github.com/mikolajgasior/gorestapi/pkg/jsonresp"
	"github.com/mikolajgasior/gorestapi/pkg/urlpath"
)

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	h.options.CORS.WriteHeaders(w)

	logAttrHandler := logger.AttrHandler(h)
	logAttrPath := logger.AttrPath(r.URL.Path)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	foundURLPath := ""
	var foundRoute Route
	for urlPath, route := range h.options.Routes {
		if strings.HasPrefix(r.URL.Path, "/"+urlPath+"/") {
			foundURLPath = urlPath
			foundRoute = route
			break
		}
	}

	if foundURLPath == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	key := foundRoute.RegistryKey
	if key == "" {
		key = foundURLPath
	}

	idPath := r.URL.Path[len("/"+foundURLPath+"/"):]
	id, ok := urlpath.ID(idPath)
	if !ok {
		slog.Debug("ID in URI not numeric", logAttrHandler, logAttrPath)
		jsonresp.Write(w, http.StatusBadRequest, &jsonresp.Response{
			Ok:   true,
			Code: jsonresp.CodeURLPathID,
		})
		return
	}

	// create and update
	if r.Method == http.MethodPut {
		if id == "" && foundRoute.Flags&DisableCreate > 0 {
			jsonresp.Write(w, http.StatusMethodNotAllowed, &jsonresp.Response{
				Ok:   true,
				Code: CodeNotAllowed,
			})
			return
		}
		if id != "" && foundRoute.Flags&DisableUpdate > 0 {
			jsonresp.Write(w, http.StatusMethodNotAllowed, &jsonresp.Response{
				Ok:   true,
				Code: CodeNotAllowed,
			})
			return
		}
		h.handleAPICreateUpdate(r.Context(), w, r, key, foundRoute, id)
		return
	}

	// delete
	if r.Method == http.MethodDelete && id != "" {
		if foundRoute.Flags&DisableDelete > 0 {
			jsonresp.Write(w, http.StatusMethodNotAllowed, &jsonresp.Response{
				Ok:   true,
				Code: CodeNotAllowed,
			})
			return
		}
		h.handleAPIDelete(r.Context(), w, r, key, foundRoute, id)
		return
	}

	// read
	if r.Method == http.MethodGet && id != "" {
		if foundRoute.Flags&DisableRead > 0 {
			jsonresp.Write(w, http.StatusMethodNotAllowed, &jsonresp.Response{
				Ok:   true,
				Code: CodeNotAllowed,
			})
			return
		}
		h.handleAPIRead(r.Context(), w, r, key, foundRoute, id)
		return
	}

	// list
	if r.Method == http.MethodGet && id == "" {
		if foundRoute.Flags&DisableList > 0 {
			jsonresp.Write(w, http.StatusMethodNotAllowed, &jsonresp.Response{
				Ok:   true,
				Code: CodeNotAllowed,
			})
			return
		}
		h.handleAPIList(r.Context(), w, r, key, foundRoute)
		return
	}

	jsonresp.Write(w, http.StatusBadRequest, &jsonresp.Response{
		Ok:   true,
		Code: jsonresp.CodeBadRequest,
	})
}
