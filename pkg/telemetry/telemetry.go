package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func NewTracerProvider(ctx context.Context, svcName string, u url.URL, instanceID string, fraction float64) (*sdktrace.TracerProvider, error) {
	if u == (url.URL{}) {
		return nil, errors.New("URL is empty")
	}
	if svcName == "" {
		return nil, errors.New("service Name is empty")
	}

	var client otlptrace.Client
	switch u.Scheme {
	case "http":
		client = otlptracehttp.NewClient(otlptracehttp.WithEndpoint(u.Host), otlptracehttp.WithURLPath(u.Path), otlptracehttp.WithInsecure())
	case "https":
		client = otlptracehttp.NewClient(otlptracehttp.WithEndpoint(u.Host), otlptracehttp.WithURLPath(u.Path))
	default:
		return nil, fmt.Errorf("unsupported tracing url scheme: %s", u.Scheme)
	}

	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(svcName),
			semconv.ServiceInstanceID(instanceID),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(fraction)),
	), nil
}

func MakeMetrics(namespace, subsystem string) (*kitprometheus.Counter, *kitprometheus.Summary) {
	counter := kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "request_count",
		Help:      "Number of requests received.",
	}, []string{"method"})
	latency := kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
		Namespace:  namespace,
		Subsystem:  subsystem,
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		Name:       "request_latency_microseconds",
		Help:       "Total duration of requests in microseconds.",
	}, []string{"method"})

	return counter, latency
}
