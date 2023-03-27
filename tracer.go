package tracer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// initiatises the OpenTelemetry client exporter
func NewTracer(
	endpointSuffix, serviceName, serviceVersion, environment string,
) (*sdktrace.TracerProvider, error) {

	// OLTP exporter
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint("ca-otel.internal." + endpointSuffix),
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
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			attribute.String("environment", environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithBatcher(stdoutExp),
		sdktrace.WithResource(res),
	), nil
}

type logWriter struct{}
type logSpan struct {
	Name string
	// Attributes []attribute.KeyValue
	Attributes json.RawMessage
	Status     trace.Status
}

// log error message (span.Status) to stdout for local debugging
func (w logWriter) Write(p []byte) (n int, err error) {

	go func() {
		// unmarshal span Status
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
