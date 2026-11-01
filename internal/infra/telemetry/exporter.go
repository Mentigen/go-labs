package telemetry

import (
	"context"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type bestEffortSpanExporter struct {
	sdktrace.SpanExporter
	report func(error)
}

func (e bestEffortSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if err := e.SpanExporter.ExportSpans(ctx, spans); err != nil {
		e.report(err)
	}
	return nil
}

type bestEffortMetricExporter struct {
	sdkmetric.Exporter
	report func(error)
}

func (e bestEffortMetricExporter) Export(ctx context.Context, metrics *metricdata.ResourceMetrics) error {
	if err := e.Exporter.Export(ctx, metrics); err != nil {
		e.report(err)
	}
	return nil
}
