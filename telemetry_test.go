package gorestapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTelemetry(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	reader := sdkmetric.NewManualReader()
	var logs bytes.Buffer
	h := postsHandler(t, Options{
		TracerProvider: tracerProvider,
		MeterProvider:  sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)),
		Logger:         slog.New(LogHandler(slog.NewJSONHandler(&logs, nil))),
		Routes: map[string]Route{"posts": {
			PostRead: func(any, *http.Request) error { return errors.New("failing hook") },
		}},
	})

	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })

	parentTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	for _, request := range []struct {
		method, path string
		traceparent  bool
	}{
		{http.MethodGet, "/posts/", true},
		{http.MethodGet, "/posts/1", false},
		{http.MethodGet, "/unknown/1", false},
		{"BREW", "/posts/", false},
	} {
		req := httptest.NewRequest(request.method, request.path, nil)
		if request.traceparent {
			req.Header.Set("traceparent", "00-"+parentTraceID+"-00f067aa0ba902b7-01")
		}
		h.Serve(httptest.NewRecorder(), req)
	}

	ended := spans.Ended()
	if len(ended) != 4 {
		t.Fatalf("the requests ended %d spans, want 4", len(ended))
	}
	for i, want := range []struct {
		name   string
		status int
		code   codes.Code
	}{
		{"GET /posts/", http.StatusOK, codes.Unset},
		{"GET /posts/{id}", http.StatusInternalServerError, codes.Error},
		{"GET", http.StatusNotFound, codes.Unset},
		{"HTTP /posts/", http.StatusMethodNotAllowed, codes.Unset},
	} {
		span := ended[i]
		attrs := map[string]string{}
		for _, attr := range span.Attributes() {
			attrs[string(attr.Key)] = attr.Value.String()
		}
		if span.Name() != want.name || attrs["http.response.status_code"] != strconv.Itoa(want.status) || span.Status().Code != want.code ||
			span.SpanKind() != trace.SpanKindServer {
			t.Fatalf("the span %d is %s with %v and the status %v, want %s with the status %d", i, span.Name(), attrs, span.Status(), want.name, want.status)
		}
	}
	if ended[0].Parent().TraceID().String() != parentTraceID || ended[1].Parent().IsValid() {
		t.Fatalf("the spans have the parents %v and %v, want the one of traceparent, and none", ended[0].Parent(), ended[1].Parent())
	}

	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &metrics); err != nil {
		t.Fatalf("Collect failed: %s", err.Error())
	}
	var requests uint64
	routes := map[string]bool{}
	for _, scope := range metrics.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			for _, point := range m.Data.(metricdata.Histogram[float64]).DataPoints {
				requests += point.Count
				route, _ := point.Attributes.Value("http.route")
				routes[route.AsString()] = true
			}
		}
	}
	if requests != 4 || !routes["/posts/"] || !routes["/posts/{id}"] || len(routes) != 3 {
		t.Fatalf("the histogram counted %d requests of the routes %v, want 4 of /posts/, /posts/{id} and none", requests, routes)
	}

	// the log of the failing hook has the IDs of the trace and the span of its request
	var record map[string]any
	if err := json.Unmarshal(bytes.SplitN(logs.Bytes(), []byte("\n"), 2)[0], &record); err != nil {
		t.Fatalf("the log %q is not JSON: %v", logs.String(), err)
	}
	if record["trace_id"] != ended[1].SpanContext().TraceID().String() || record["span_id"] != ended[1].SpanContext().SpanID().String() || record["request_id"] == nil {
		t.Fatalf("the log of the hook is %v, want the trace, the span and the request of the read", record)
	}
}
