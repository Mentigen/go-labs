package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Mentigen/go-labs/internal/config"
)

const (
	serviceNamespace  = "tripgo"
	environmentName   = "local"
	errorLogThrottle  = 30 * time.Second
	unknownBuildLabel = "dev"

	defaultMetricInterval = 15 * time.Second
)

type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Propagator     propagation.TextMapPropagator
	ReportError    func(error)
}

func Setup(ctx context.Context, cfg config.TelemetryConfig, logger *slog.Logger) (*Telemetry, error) {
	report := newErrorHandler(logger).Handle

	res, err := newResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	traceExporter, metricExporter, err := newExporters(ctx, cfg.Protocol)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(bestEffortSpanExporter{SpanExporter: traceExporter, report: report}),
		sdktrace.WithResource(res),
	)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			bestEffortMetricExporter{Exporter: metricExporter, report: report}, metricReaderOptions()...,
		)),
		sdkmetric.WithResource(res),
	)

	return &Telemetry{
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
		ReportError:    report,
		Propagator: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	return errors.Join(
		t.TracerProvider.Shutdown(ctx),
		t.MeterProvider.Shutdown(ctx),
	)
}

func metricReaderOptions() []sdkmetric.PeriodicReaderOption {
	if os.Getenv("OTEL_METRIC_EXPORT_INTERVAL") != "" {
		return nil
	}

	return []sdkmetric.PeriodicReaderOption{sdkmetric.WithInterval(defaultMetricInterval)}
}

func newExporters(
	ctx context.Context,
	protocol string,
) (sdktrace.SpanExporter, sdkmetric.Exporter, error) {
	if protocol == config.ProtocolGRPC {
		traceExporter, err := otlptracegrpc.New(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("create grpc trace exporter: %w", err)
		}

		metricExporter, err := otlpmetricgrpc.New(ctx)
		if err != nil {
			_ = traceExporter.Shutdown(ctx)
			return nil, nil, fmt.Errorf("create grpc metric exporter: %w", err)
		}

		return traceExporter, metricExporter, nil
	}

	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("create http trace exporter: %w", err)
	}

	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		return nil, nil, fmt.Errorf("create http metric exporter: %w", err)
	}

	return traceExporter, metricExporter, nil
}

func newResource(ctx context.Context, cfg config.TelemetryConfig) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.version", buildVersion()),
			attribute.String("service.namespace", serviceNamespace),
			attribute.String("deployment.environment", environmentName),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return unknownBuildLabel
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && len(setting.Value) >= 7 {
			return setting.Value[:7]
		}
	}

	return unknownBuildLabel
}

func newErrorHandler(logger *slog.Logger) otel.ErrorHandler {
	var lastLoggedAt atomic.Int64

	return otel.ErrorHandlerFunc(func(err error) {
		now := time.Now().UnixNano()
		last := lastLoggedAt.Load()

		if now-last < int64(errorLogThrottle) || !lastLoggedAt.CompareAndSwap(last, now) {
			return
		}

		logger.Warn("telemetry error", slog.String("error", err.Error()))
	})
}
