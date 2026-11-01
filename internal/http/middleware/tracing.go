package middleware

import (
	"net/http"
	"strconv"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const errorTypeKey = attribute.Key("error.type")

func Tracing(
	tracerProvider trace.TracerProvider,
	propagator propagation.TextMapPropagator,
	resolve RouteResolver,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span := trace.SpanFromContext(r.Context())
			if state, ok := r.Context().Value(recoveryKey{}).(*recoveryState); ok {
				state.spanContext = span.SpanContext()
			}
			route := resolve(r)

			if route != "" {
				span.SetAttributes(semconv.HTTPRoute(route))
			}

			track(w, r, next, route, func(result requestResult) {
				span.SetAttributes(semconv.HTTPResponseStatusCode(result.status))
				if result.status >= http.StatusInternalServerError {
					span.SetStatus(codes.Error, http.StatusText(result.status))
					span.SetAttributes(errorTypeKey.String(strconv.Itoa(result.status)))
				}
			})
		})

		return otelhttp.NewHandler(inner, "http.server",
			otelhttp.WithTracerProvider(tracerProvider),
			otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()),
			otelhttp.WithPropagators(propagator),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return spanName(r.Method, resolve(r))
			}),
		)
	}
}

func spanName(method, route string) string {
	if route == "" {
		return method
	}

	return method + " " + route
}
