package gorestapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/semconv/v1.43.0/httpconv"
	"go.opentelemetry.io/otel/trace"
)

const telemetryScope = "github.com/mikogs/gorestapi"

// telemetry has the instruments of OpenTelemetry of the handler, by the semantic conventions of HTTP servers.
type telemetry struct {
	tracer   trace.Tracer
	duration httpconv.ServerRequestDuration
}

func newTelemetry(tracerProvider trace.TracerProvider, meterProvider metric.MeterProvider) *telemetry {
	if tracerProvider == nil {
		tracerProvider = otel.GetTracerProvider()
	}
	if meterProvider == nil {
		meterProvider = otel.GetMeterProvider()
	}
	// the instrument fails only with a meter that rejects its name, and the requests are served without it then
	duration, _ := httpconv.NewServerRequestDuration(meterProvider.Meter(telemetryScope))
	return &telemetry{
		tracer:   tracerProvider.Tracer(telemetryScope),
		duration: duration,
	}
}

// startSpan starts the server span of r, a child of the span of the trace context of its headers, e.g. traceparent,
// named by the method and the route, e.g. GET /notes/{id}, as the path has the IDs. The propagator is the global one
// of each request, as applications often set it after they create the handler.
func (t *telemetry) startSpan(ctx context.Context, r *http.Request, route string) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
	// a method that is not of HTTP is named HTTP, so that clients cannot add names of spans
	method := requestMethod(r.Method)
	name := string(method)
	if method == httpconv.RequestMethodOther {
		name = "HTTP"
	}
	if route != "" {
		name += " " + route
	}
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(string(method)),
		semconv.URLPath(requestPath(r)),
		semconv.URLScheme(urlScheme(r)),
		semconv.UserAgentOriginal(r.UserAgent()),
	}
	if route != "" {
		attrs = append(attrs, semconv.HTTPRoute(route))
	}
	return t.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attrs...))
}

// endSpan ends the span of a request, and records its duration, with the status of its response. A status of 5xx is an
// error of the server, and its span has the status Error, while a status of 4xx is an error of the client.
func (t *telemetry) endSpan(ctx context.Context, span trace.Span, r *http.Request, route string, status int, start time.Time) {
	attrs := []attribute.KeyValue{t.duration.AttrResponseStatusCode(status)}
	if route != "" {
		attrs = append(attrs, t.duration.AttrRoute(route))
	}
	span.SetAttributes(semconv.HTTPResponseStatusCode(status))
	if status >= http.StatusInternalServerError {
		errorType := strconv.Itoa(status)
		attrs = append(attrs, t.duration.AttrErrorType(httpconv.ErrorTypeAttr(errorType)))
		span.SetAttributes(semconv.ErrorTypeKey.String(errorType))
		span.SetStatus(codes.Error, http.StatusText(status))
	}
	t.duration.Record(ctx, time.Since(start).Seconds(), requestMethod(r.Method), urlScheme(r), attrs...)
	span.End()
}

// httpRoute returns the route of r for the telemetry, e.g. /notes/{id} for /notes/42, or an empty string for a path of
// no route.
func (h *Handler) httpRoute(r *http.Request) string {
	urlPath, _ := h.findRoute(r)
	if urlPath == "" {
		return ""
	}
	if r.URL.Path == "/"+urlPath+"/" {
		return "/" + urlPath + "/"
	}
	return "/" + urlPath + "/{id}"
}

// requestMethod returns the method of a request, or _OTHER for a method that is not of HTTP, so that the methods of
// the clients do not add values of the attribute.
func requestMethod(method string) httpconv.RequestMethodAttr {
	switch method {
	case http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodOptions, http.MethodPost, http.MethodPatch,
		http.MethodHead, http.MethodConnect, http.MethodTrace:
		return httpconv.RequestMethodAttr(method)
	}
	return httpconv.RequestMethodOther
}

func urlScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
