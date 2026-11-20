package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	operationDurationName = "db.client.operation.duration"
	errorTypeKey          = attribute.Key("error.type")
)

var durationBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type QueryTracer struct {
	*multitracer.Tracer
}

func NewQueryTracer(tracerProvider trace.TracerProvider, meterProvider metric.MeterProvider) (*QueryTracer, error) {
	duration, err := meterProvider.Meter("github.com/Mentigen/go-labs/internal/infra/postgres").Float64Histogram(
		operationDurationName,
		metric.WithUnit("s"),
		metric.WithDescription("Duration of database client operations."),
		metric.WithExplicitBucketBoundaries(durationBuckets...),
	)
	if err != nil {
		return nil, err
	}

	spans := otelpgx.NewTracer(
		otelpgx.WithTracerProvider(tracerProvider),
		otelpgx.WithMeterProvider(metricnoop.NewMeterProvider()),
		otelpgx.WithDisableConnectionDetailsInAttributes(),
	)

	return &QueryTracer{Tracer: multitracer.New(spans, &operationMetrics{duration: duration})}, nil
}

func (t *QueryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	for i := len(t.QueryTracers) - 1; i >= 0; i-- {
		t.QueryTracers[i].TraceQueryEnd(ctx, conn, data)
	}
}

type operationStartKey struct{}

type operationStart struct {
	startedAt  time.Time
	operation  string
	collection string
}

type operationMetrics struct {
	duration metric.Float64Histogram
}

func (m *operationMetrics) TraceQueryStart(
	ctx context.Context,
	_ *pgx.Conn,
	data pgx.TraceQueryStartData,
) context.Context {
	operation, collection := parseStatement(data.SQL)

	if collection != "" {
		trace.SpanFromContext(ctx).SetAttributes(semconv.DBCollectionName(collection))
	}

	return context.WithValue(ctx, operationStartKey{}, operationStart{
		startedAt:  time.Now(),
		operation:  operation,
		collection: collection,
	})
}

func (m *operationMetrics) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(operationStartKey{}).(operationStart)
	if !ok {
		return
	}

	attrs := []attribute.KeyValue{
		semconv.DBSystemNamePostgreSQL,
		semconv.DBOperationName(start.operation),
	}

	if start.collection != "" {
		attrs = append(attrs, semconv.DBCollectionName(start.collection))
	}

	if data.Err != nil {
		errorType := errorTypeOf(data.Err)
		attrs = append(attrs, errorTypeKey.String(errorType))
		trace.SpanFromContext(ctx).SetAttributes(errorTypeKey.String(errorType))
	}

	m.duration.Record(ctx, time.Since(start.startedAt).Seconds(), metric.WithAttributes(attrs...))
}

func errorTypeOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	if errors.Is(err, context.Canceled) {
		return "canceled"
	}

	return "_OTHER"
}

func parseStatement(statement string) (operation, collection string) {
	words := strings.Fields(statement)
	if len(words) == 0 {
		return "UNKNOWN", ""
	}

	operation = strings.ToUpper(words[0])

	marker := "FROM"

	switch operation {
	case "INSERT":
		marker = "INTO"
	case "UPDATE":
		collection = tableName(words, 0)
		return operation, collection
	}

	for index, word := range words {
		if strings.EqualFold(word, marker) {
			return operation, tableName(words, index)
		}
	}

	return operation, ""
}

func tableName(words []string, markerIndex int) string {
	if markerIndex+1 >= len(words) {
		return ""
	}

	return strings.Trim(words[markerIndex+1], `"();,`)
}
