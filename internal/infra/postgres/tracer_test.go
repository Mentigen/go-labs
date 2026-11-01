package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Mentigen/go-labs/internal/infra/postgres"
)

func TestQueryFailureRecordsSQLStateBeforeEndingSpan(t *testing.T) {
	t.Parallel()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})

	tracer, err := postgres.NewQueryTracer(tp, mp)
	if err != nil {
		t.Fatal(err)
	}

	ctx, parent := tp.Tracer("test").Start(context.Background(), "request")
	defer parent.End()
	ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "INSERT INTO trips (id) VALUES ($1)"})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: &pgconn.PgError{Code: "23505"}})

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("DB spans = %d, want 1", len(spans))
	}
	span := spans[0]
	attrs := attribute.NewSet(span.Attributes...)
	assertQueryAttributes(t, attrs)
	if span.Status.Code != codes.Error || !span.Parent.Equal(parent.SpanContext()) {
		t.Errorf("DB span must be an error under the request: %+v", span)
	}

	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &metrics); err != nil {
		t.Fatal(err)
	}
	var count uint64
	for _, scope := range metrics.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			if instrument.Name != "db.client.operation.duration" {
				continue
			}
			if instrument.Unit != "s" {
				t.Errorf("DB duration unit = %q, want seconds", instrument.Unit)
			}
			for _, point := range instrument.Data.(metricdata.Histogram[float64]).DataPoints {
				assertQueryAttributes(t, point.Attributes)
				count += point.Count
			}
		}
	}
	if count != 1 {
		t.Errorf("DB operations recorded = %d, want 1", count)
	}
}

func assertQueryAttributes(t *testing.T, attrs attribute.Set) {
	t.Helper()
	for key, want := range map[attribute.Key]string{
		"db.operation.name":  "INSERT",
		"db.collection.name": "trips",
		"error.type":         "23505",
	} {
		value, ok := attrs.Value(key)
		if !ok || value.AsString() != want {
			t.Errorf("%s = %v, want %q", key, value, want)
		}
	}
}
