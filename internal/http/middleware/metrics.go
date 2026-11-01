package middleware

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

const meterName = "github.com/Mentigen/go-labs/internal/http/middleware"

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

func Metrics(provider metric.MeterProvider, resolve RouteResolver) (func(http.Handler) http.Handler, error) {
	meter := provider.Meter(meterName)

	duration, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of HTTP server requests."),
		metric.WithExplicitBucketBoundaries(durationBuckets...),
	)
	if err != nil {
		return nil, fmt.Errorf("create request duration histogram: %w", err)
	}

	active, err := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Number of in-flight HTTP server requests."),
	)
	if err != nil {
		return nil, fmt.Errorf("create active requests counter: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			route := resolve(r)

			activeAttrs := metric.WithAttributes(requestAttributes(r.Method, route)...)

			active.Add(ctx, 1, activeAttrs)
			defer active.Add(ctx, -1, activeAttrs)

			track(w, r, next, route, func(result requestResult) {
				attrs := append(requestAttributes(r.Method, route), semconv.HTTPResponseStatusCode(result.status))

				duration.Record(ctx, result.duration.Seconds(), metric.WithAttributes(attrs...))
			})
		})
	}, nil
}

func requestAttributes(method, route string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{semconv.HTTPRequestMethodKey.String(method)}

	if route != "" {
		attrs = append(attrs, semconv.HTTPRoute(route))
	}

	return attrs
}
