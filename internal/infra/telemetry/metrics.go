package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/metric"
)

const meterName = "github.com/Mentigen/go-labs/internal/infra/telemetry"

type TripMetrics struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func NewTripMetrics(provider metric.MeterProvider) (*TripMetrics, error) {
	meter := provider.Meter(meterName)

	created, err := meter.Int64Counter(
		"tripgo.trips.created",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Number of created trips."),
	)
	if err != nil {
		return nil, fmt.Errorf("create trips.created counter: %w", err)
	}

	completed, err := meter.Int64Counter(
		"tripgo.trips.completed",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Number of completed trips."),
	)
	if err != nil {
		return nil, fmt.Errorf("create trips.completed counter: %w", err)
	}

	return &TripMetrics{created: created, completed: completed}, nil
}

func (m *TripMetrics) TripCreated(ctx context.Context) {
	m.created.Add(ctx, 1)
}

func (m *TripMetrics) TripCompleted(ctx context.Context) {
	m.completed.Add(ctx, 1)
}

func RegisterActiveTrips(
	provider metric.MeterProvider,
	count func(ctx context.Context) (int64, error),
	timeout time.Duration,
	report func(error),
) error {
	_, err := provider.Meter(meterName).Int64ObservableGauge(
		"tripgo.trips.active",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Number of trips in active status."),
		metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			active, err := count(ctx)
			if err != nil {
				report(fmt.Errorf("count active trips: %w", err))
				return nil
			}

			observer.Observe(active)

			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("create trips.active gauge: %w", err)
	}

	return nil
}
