package gorestapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

// CodeServiceUnavailable is the code of the problem of a readiness probe whose check fails.
const CodeServiceUnavailable = "SERVICE_UNAVAILABLE"

// Liveness is the handler of a probe of liveness, e.g. /healthz of Kubernetes: it responds 200 while the process
// serves requests. It does not check the database, as a restart of the process would not fix it.
func Liveness(w http.ResponseWriter, _ *http.Request) {
	jsonresp.Write(w, http.StatusOK, &jsonresp.Response{Ok: true, Code: jsonresp.CodeSuccess})
}

// Readiness returns the handler of a probe of readiness, e.g. /readyz of Kubernetes: it responds 200 when check
// returns nil, e.g. db.PingContext, and the problem SERVICE_UNAVAILABLE otherwise, so that no requests are sent to the
// process until it can serve them. The check gets the context of the request, with the Timeout of the options.
func (h *Handler) Readiness(check func(ctx context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if timeout := orDefault(h.options.Timeout, DefaultTimeout); timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		if err := check(ctx); err != nil {
			h.log().LogAttrs(ctx, slog.LevelWarn, "readiness check failed", handlerAttr, slog.String("err", err.Error()))
			h.writeProblem(w, r, CodeServiceUnavailable, "", nil)
			return
		}
		jsonresp.Write(w, http.StatusOK, &jsonresp.Response{Ok: true, Code: jsonresp.CodeSuccess})
	}
}
