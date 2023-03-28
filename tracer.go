package instrumentation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

type ShutdownFunc func()

// set instrumentation library name
func init() {
	tracer = otel.Tracer("credibil/instrumentation")
}

// Tracer is http middleware that adds OpenTelemetry tracing to each request.
func Tracer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), r.URL.Path, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// WithTracer initiatises OpenTelemetry trace exporter(s)
func WithTracer() (shutdown ShutdownFunc, err error) {

	// create OLTP http exporter
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint("ca-otel.internal." + os.Getenv("CONTAINER_APP_ENV_DNS_SUFFIX")),
		// otlptracehttp.WithInsecure(),
	)
	traceExp, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	// stdout exporter
	stdoutExp, err := stdouttrace.New(stdouttrace.WithWriter(logWriter{}))
	if err != nil {
		return nil, fmt.Errorf("creating stdout exporter: %w", err)
	}

	// define this resource
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(os.Getenv("CONTAINER_APP_NAME")),
			semconv.ServiceVersion(os.Getenv("CONTAINER_APP_REVISION")),
			attribute.String("environment", os.Getenv("CONTAINER_APP_ENV_DNS_SUFFIX")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	// create trace provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithBatcher(stdoutExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// return shutdown function
	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("error shutting down trace provider: %+v", err)
		}
	}, nil
}

// Implement a custom log writer to simplify log entries from the stdout exporter
type logWriter struct{}
type logSpan struct {
	Name   string
	Status sdktrace.Status
	// Attributes []attribute.KeyValue
}

// Write logs the span.Status description for error spans
func (w logWriter) Write(p []byte) (n int, err error) {
	go func() {
		var span logSpan
		if err := json.Unmarshal(p, &span); err != nil {
			log.Printf("error unmarshaling span: %v", err)
		}
		if span.Status.Code == codes.Error {
			log.Printf("error in %s: %s", span.Name, span.Status.Description)
		}
	}()
	return len(p), nil
}
