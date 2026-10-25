package gorestapi

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// RequestIDHeader is the header of the ID of a request, which the handler reads from the request, or generates, and
// writes to the response.
const RequestIDHeader = "X-Request-ID"

// regexpRequestID matches the IDs of requests from clients and proxies that are used as they are: short, and without
// characters that could change the lines of the logs.
var regexpRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type requestIDKey struct{}

// RequestID returns the ID of the request of ctx, or an empty string for a context of no request of the handler.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// requestID returns the ID of the request of header, and writes it to the response: the ID of the RequestIDHeader of
// the request, or a new one when it has none, or one that cannot be logged as it is.
func requestID(w http.ResponseWriter, header http.Header) string {
	id := header.Get(RequestIDHeader)
	if !regexpRequestID.MatchString(id) {
		id = rand.Text()
	}
	w.Header().Set(RequestIDHeader, id)
	return id
}

// LogHandler returns a handler of slog that adds the request_id attribute, the ID of the request of the context, to
// the records of next, e.g. of the handler, of the registry and of the hooks, which log with the context of the
// request, and the trace_id and span_id attributes of its span of OpenTelemetry, when it is recorded.
func LogHandler(next slog.Handler) slog.Handler {
	return requestIDHandler{next}
}

type requestIDHandler struct {
	slog.Handler
}

func (h requestIDHandler) Handle(ctx context.Context, record slog.Record) error {
	id := RequestID(ctx)
	spanContext := trace.SpanContextFromContext(ctx)
	if id == "" && !spanContext.IsValid() {
		return h.Handler.Handle(ctx, record)
	}
	record = record.Clone()
	if id != "" {
		record.AddAttrs(slog.String("request_id", id))
	}
	if spanContext.IsValid() {
		record.AddAttrs(slog.String("trace_id", spanContext.TraceID().String()), slog.String("span_id", spanContext.SpanID().String()))
	}
	return h.Handler.Handle(ctx, record)
}

func (h requestIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestIDHandler{h.Handler.WithAttrs(attrs)}
}

func (h requestIDHandler) WithGroup(name string) slog.Handler {
	return requestIDHandler{h.Handler.WithGroup(name)}
}

// responseRecorder records the status and the size of a response, for the telemetry and the access log.
type responseRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

// statusOrOK returns the status of the response, or 200 for a response without a status and a body, as net/http writes
// it.
func (r *responseRecorder) statusOrOK() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(data)
	r.size += n
	return n, err
}

// logAccess logs a request that the handler served, with the status and the size of its response, and its duration.
func (h *Handler) logAccess(r *http.Request, recorder *responseRecorder, start time.Time) {
	h.log().LogAttrs(r.Context(), slog.LevelInfo, "request", handlerAttr,
		slog.String("method", r.Method),
		slog.String("route", h.RoutePath(r)),
		slog.String("path", requestPath(r)),
		slog.String("client", ClientIP(r, h.options.TrustedProxies).String()),
		slog.Int("status", recorder.statusOrOK()),
		slog.Int("size", recorder.size),
		slog.Duration("duration", time.Since(start)),
	)
}
