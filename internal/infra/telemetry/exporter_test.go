package telemetry

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"
)

//go:generate go tool mockgen -destination span_mocks_test.go -package telemetry go.opentelemetry.io/otel/sdk/trace SpanExporter
//go:generate go tool mockgen -destination metric_mocks_test.go -package telemetry go.opentelemetry.io/otel/sdk/metric Exporter

func TestSpanExportFailureIsHandledLocally(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	failure := errors.New("collector unavailable")
	backend := NewMockSpanExporter(gomock.NewController(t))
	backend.EXPECT().ExportSpans(ctx, nil).Return(failure)
	backend.EXPECT().Shutdown(ctx).Return(failure)
	var reported error
	exporter := bestEffortSpanExporter{SpanExporter: backend, report: func(err error) { reported = err }}
	if err := exporter.ExportSpans(ctx, nil); err != nil || !errors.Is(reported, failure) {
		t.Fatalf("background failure must be reported locally: returned=%v reported=%v", err, reported)
	}
	if err := exporter.Shutdown(ctx); !errors.Is(err, failure) {
		t.Fatalf("shutdown must preserve failure: %v", err)
	}
}

func TestMetricExportFailureIsHandledLocally(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	failure := errors.New("collector unavailable")
	metrics := &metricdata.ResourceMetrics{}
	backend := NewMockExporter(gomock.NewController(t))
	backend.EXPECT().Export(ctx, metrics).Return(failure)
	backend.EXPECT().ForceFlush(ctx).Return(failure)
	backend.EXPECT().Shutdown(ctx).Return(failure)
	var reported error
	exporter := bestEffortMetricExporter{Exporter: backend, report: func(err error) { reported = err }}
	if err := exporter.Export(ctx, metrics); err != nil || !errors.Is(reported, failure) {
		t.Fatalf("background failure must be reported locally: returned=%v reported=%v", err, reported)
	}
	if err := exporter.ForceFlush(ctx); !errors.Is(err, failure) {
		t.Fatalf("flush must preserve failure: %v", err)
	}
	if err := exporter.Shutdown(ctx); !errors.Is(err, failure) {
		t.Fatalf("shutdown must preserve failure: %v", err)
	}
}
