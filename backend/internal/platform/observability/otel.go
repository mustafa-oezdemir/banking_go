// Package observability wires safe OpenTelemetry trace propagation at service edges.
package observability

import (
	"context"
	"net/http"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Init installs a process-local provider. An OTLP endpoint is optional so unit
// tests and local binaries remain runnable without a collector.
func Init(ctx context.Context, service string) func(context.Context) error {
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", service))),
	}
	if endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")); endpoint != "" {
		exporterOptions := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint)}
		if strings.EqualFold(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"), "true") {
			exporterOptions = append(exporterOptions, otlptracehttp.WithInsecure())
		}
		if exporter, err := otlptracehttp.New(ctx, exporterOptions...); err == nil {
			options = append(options, sdktrace.WithBatcher(exporter))
		}
	}
	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return provider.Shutdown
}

// HTTP extracts W3C trace context, starts a server span, and returns only a
// non-sensitive trace identifier for safe correlation with structured logs.
func HTTP(service string, next http.Handler) http.Handler {
	tracer := otel.Tracer(service)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(request.Context(), propagation.HeaderCarrier(request.Header))
		ctx, span := tracer.Start(ctx, request.Method+" "+request.URL.Path)
		defer span.End()
		span.SetAttributes(attribute.String("http.request.method", request.Method), attribute.String("url.path", request.URL.Path))
		if traceID := span.SpanContext().TraceID(); traceID.IsValid() {
			w.Header().Set("X-Trace-ID", traceID.String())
		}
		next.ServeHTTP(w, request.WithContext(ctx))
	})
}
