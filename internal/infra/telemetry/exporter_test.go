package telemetry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
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
	{
		err := exporter.ExportSpans(ctx, nil)
		require.NoError(t, err)
		require.ErrorIs(t, reported, failure)
	}
	require.ErrorIs(t, exporter.Shutdown(ctx), failure)
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
	{
		err := exporter.Export(ctx, metrics)
		require.NoError(t, err)
		require.ErrorIs(t, reported, failure)
	}
	require.ErrorIs(t, exporter.ForceFlush(ctx), failure)
	require.ErrorIs(t, exporter.Shutdown(ctx), failure)
}
