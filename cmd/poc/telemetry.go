package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// setupTelemetry sets the global providers of OpenTelemetry: of the metrics, which the returned handler serves at
// /metrics for Prometheus, with the statistics of the connections of db, and of the traces, which are exported with
// OTLP over HTTP when OTEL_EXPORTER_OTLP_ENDPOINT is set, e.g. to a collector or Jaeger. shutdown exports the last
// spans.
func setupTelemetry(ctx context.Context, db *sql.DB) (metrics http.Handler, shutdown func(context.Context) error, err error) {
	// the variables of OpenTelemetry, e.g. OTEL_SERVICE_NAME, replace the name of the service
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName("gorestapi-poc")), resource.WithFromEnv())
	if err != nil {
		return nil, nil, err
	}

	registry := prometheus.NewRegistry()
	if err := registry.Register(collectors.NewDBStatsCollector(db, "poc")); err != nil {
		return nil, nil, err
	}
	exporter, err := otelprometheus.New(otelprometheus.WithRegisterer(registry))
	if err != nil {
		return nil, nil, err
	}
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter), sdkmetric.WithResource(res))
	otel.SetMeterProvider(meterProvider)

	tracerOptions := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		spanExporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, nil, err
		}
		tracerOptions = append(tracerOptions, sdktrace.WithBatcher(spanExporter))
	}
	tracerProvider := sdktrace.NewTracerProvider(tracerOptions...)
	otel.SetTracerProvider(tracerProvider)
	// the trace context of the requests, e.g. of a gateway, makes their spans children of its spans
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	shutdown = func(ctx context.Context) error {
		return errors.Join(tracerProvider.Shutdown(ctx), meterProvider.Shutdown(ctx))
	}
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), shutdown, nil
}
