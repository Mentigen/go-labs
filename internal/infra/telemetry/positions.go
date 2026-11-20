package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type PositionMetrics struct {
	saved, checks, requests metric.Int64Counter
	rows                    metric.Int64Histogram
	duration                metric.Float64Histogram
}

func NewPositionMetrics(provider metric.MeterProvider) (*PositionMetrics, error) {
	meter := provider.Meter(meterName)
	m := &PositionMetrics{}
	var err error
	m.saved, err = meter.Int64Counter("tripgo.positions.saved")
	if err != nil {
		return nil, fmt.Errorf("create saved positions counter: %w", err)
	}
	m.checks, err = meter.Int64Counter("tripgo.stale_checks")
	if err != nil {
		return nil, fmt.Errorf("create stale checks counter: %w", err)
	}
	m.requests, err = meter.Int64Counter("tripgo.position_requests")
	if err != nil {
		return nil, fmt.Errorf("create position requests counter: %w", err)
	}
	m.rows, err = meter.Int64Histogram("tripgo.stale_checks.rows_read")
	if err != nil {
		return nil, fmt.Errorf("create scan rows histogram: %w", err)
	}
	m.duration, err = meter.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10))
	if err != nil {
		return nil, fmt.Errorf("create push duration histogram: %w", err)
	}
	return m, nil
}

func (m *PositionMetrics) PositionSaved(ctx context.Context) { m.saved.Add(ctx, 1) }
func (m *PositionMetrics) StaleCheck(ctx context.Context, result string) {
	m.checks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *PositionMetrics) RowsRead(ctx context.Context, rows int) { m.rows.Record(ctx, int64(rows)) }

func (m *PositionMetrics) RequestFinished(ctx context.Context, result string) {
	m.requests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *PositionMetrics) RequestDuration(ctx context.Context, result string, duration time.Duration) {
	m.duration.Record(ctx, duration.Seconds(), metric.WithAttributes(attribute.String("result", result)))
}

func RegisterPositionPool(provider metric.MeterProvider, size func() int, active func() int64) error {
	meter := provider.Meter(meterName)
	for _, instrument := range []struct {
		name  string
		value func() int64
	}{
		{"tripgo.position_requests.queue.size", func() int64 { return int64(size()) }},
		{"tripgo.position_requests.workers.active", active},
	} {
		_, err := meter.Int64ObservableGauge(instrument.name, metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			o.Observe(instrument.value())
			return nil
		}))
		if err != nil {
			return fmt.Errorf("create %s: %w", instrument.name, err)
		}
	}
	return nil
}
