package gorestapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mikogs/gorestapi/pkg/jsonresp"
	"github.com/mikogs/gorestapi/pkg/urlpath"
)

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	route := h.httpRoute(r)
	ctx := context.WithValue(r.Context(), requestIDKey{}, requestID(w, r.Header))
	ctx, span := h.telemetry.startSpan(ctx, r, route)
	// the queries are cancelled after the timeout, also when the client does not close the connection
	if timeout := orDefault(h.options.Timeout, DefaultTimeout); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	r = r.WithContext(ctx)

	recorder := &responseRecorder{ResponseWriter: w}
	w = recorder
	defer func() {
		h.telemetry.endSpan(ctx, span, r, route, recorder.statusOrOK(), start)
		if h.options.AccessLog {
			h.logAccess(r, recorder, start)
		}
	}()

	h.writeSecurityHeaders(w)

	foundURLPath, foundRoute := h.findRoute(r)

	if foundURLPath == "" {
		h.writeProblem(w, r, jsonresp.CodeNotFound, "There is no route for the URL.", nil)
		return
	}

	key := routeKey(foundURLPath, foundRoute)

	idPath := r.URL.Path[len("/"+foundURLPath+"/"):]
	id, ok := urlpath.ID(idPath)
	if !ok {
		h.log().LogAttrs(ctx, slog.LevelDebug, "invalid ID in the URL", handlerAttr, slog.String("path", r.URL.Path))
		h.writeProblem(w, r, jsonresp.CodeURLPathID, "", nil)
		return
	}

	// the preflights of CORS are answered by a middleware of CORS in front of the handler, see the docs
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", strings.Join(allowedMethods(foundRoute, id), ", "))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx, authenticated := h.authenticate(ctx, w, r, foundRoute)
	if !authenticated {
		return
	}
	r = r.WithContext(ctx)
	if !h.rateLimit(ctx, w, r) {
		return
	}

	// create and update
	if r.Method == http.MethodPut {
		if id == "" && foundRoute.Flags&DisableCreate > 0 {
			h.writeNotAllowed(w, r, foundRoute, id)
			return
		}
		if id != "" && foundRoute.Flags&DisableUpdate > 0 {
			h.writeNotAllowed(w, r, foundRoute, id)
			return
		}
		h.handleAPICreateUpdate(ctx, w, r, key, foundRoute, id)
		return
	}

	// delete
	if r.Method == http.MethodDelete && id != "" {
		if foundRoute.Flags&DisableDelete > 0 {
			h.writeNotAllowed(w, r, foundRoute, id)
			return
		}
		h.handleAPIDelete(ctx, w, r, key, foundRoute, id)
		return
	}

	// read
	if r.Method == http.MethodGet && id != "" {
		if foundRoute.Flags&DisableRead > 0 {
			h.writeNotAllowed(w, r, foundRoute, id)
			return
		}
		h.handleAPIRead(ctx, w, r, key, foundRoute, id)
		return
	}

	// list
	if r.Method == http.MethodGet && id == "" {
		if foundRoute.Flags&DisableList > 0 {
			h.writeNotAllowed(w, r, foundRoute, id)
			return
		}
		h.handleAPIList(ctx, w, r, key, foundRoute)
		return
	}

	h.writeNotAllowed(w, r, foundRoute, id)
}

// writeNotAllowed writes the problem of a method that the route does not allow for the URL, with the methods that it
// allows in the Allow header, as HTTP requires.
func (h *Handler) writeNotAllowed(w http.ResponseWriter, r *http.Request, route Route, id string) {
	w.Header().Set("Allow", strings.Join(allowedMethods(route, id), ", "))
	h.writeProblem(w, r, CodeNotAllowed, "", nil)
}

// allowedMethods returns the methods that route allows for the URL of a list, or of a record when id is set.
func allowedMethods(route Route, id string) []string {
	methods := []string{http.MethodOptions}
	if id == "" {
		if route.Flags&DisableList == 0 {
			methods = append(methods, http.MethodGet)
		}
		if route.Flags&DisableCreate == 0 {
			methods = append(methods, http.MethodPut)
		}
	} else {
		if route.Flags&DisableRead == 0 {
			methods = append(methods, http.MethodGet)
		}
		if route.Flags&DisableUpdate == 0 {
			methods = append(methods, http.MethodPut)
		}
		if route.Flags&DisableDelete == 0 {
			methods = append(methods, http.MethodDelete)
		}
	}
	return methods
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
