package gorestapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mikogs/gorestapi/pkg/jsonresp"
	"github.com/mikogs/gorestapi/pkg/urlpath"
)

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	h.options.CORS.WriteHeaders(w)

	// the queries are cancelled after the timeout, also when the client does not close the connection
	if timeout := orDefault(h.options.Timeout, DefaultTimeout); timeout > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		r = r.WithContext(ctx)
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	foundURLPath, foundRoute := h.findRoute(r)

	if foundURLPath == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	key := routeKey(foundURLPath, foundRoute)

	idPath := r.URL.Path[len("/"+foundURLPath+"/"):]
	id, ok := urlpath.ID(idPath)
	if !ok {
		h.log().LogAttrs(r.Context(), slog.LevelDebug, "invalid ID in the URL", handlerAttr, slog.String("path", r.URL.Path))
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

// RoutePath returns the path of the route of r, a key of Options.Routes, e.g. "notes" for /notes/42, or an empty
// string when r is not for any of them. It has few values, e.g. for the labels of the metrics of the requests, unlike
// the path of r, which has the IDs, and any path that a client sends.
func (h *Handler) RoutePath(r *http.Request) string {
	urlPath, _ := h.findRoute(r)
	return urlPath
}

func (h *Handler) findRoute(r *http.Request) (string, Route) {
	for urlPath, route := range h.options.Routes {
		if strings.HasPrefix(r.URL.Path, "/"+urlPath+"/") {
			return urlPath, route
		}
	}
	return "", Route{}
}
