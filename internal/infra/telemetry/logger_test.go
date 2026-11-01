package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Mentigen/go-labs/internal/infra/telemetry"
)

func decode(t *testing.T, buffer *bytes.Buffer) map[string]any {
	t.Helper()

	record := map[string]any{}
	if err := json.Unmarshal(bytes.TrimSpace(buffer.Bytes()), &record); err != nil {
		t.Fatalf("log line is not JSON: %q", buffer.String())
	}

	return record
}

func TestLoggerAddsTraceIdentifiersInsideSpan(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := telemetry.NewLogger(&buffer, slog.LevelInfo, "trip-service")
	provider := sdktrace.NewTracerProvider()

	ctx, span := provider.Tracer("test").Start(context.Background(), "operation")
	defer span.End()

	logger.InfoContext(ctx, "inside span")

	record := decode(t, &buffer)

	if record["trace_id"] != span.SpanContext().TraceID().String() {
		t.Errorf("trace_id = %v, want %v", record["trace_id"], span.SpanContext().TraceID())
	}

	if record["span_id"] != span.SpanContext().SpanID().String() {
		t.Errorf("span_id = %v, want %v", record["span_id"], span.SpanContext().SpanID())
	}
}

func TestLoggerWritesRequiredFieldsWithoutSpan(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := telemetry.NewLogger(&buffer, slog.LevelInfo, "trip-service")

	logger.With("component", "test").InfoContext(context.Background(), "outside span")

	record := decode(t, &buffer)

	for _, field := range []string{"time", "level", "msg", "service"} {
		if _, ok := record[field]; !ok {
			t.Errorf("field %q is missing: %v", field, record)
		}
	}

	if record["service"] != "trip-service" || record["component"] != "test" {
		t.Errorf("unexpected record: %v", record)
	}

	if _, ok := record["trace_id"]; ok {
		t.Errorf("trace_id must be absent outside a span: %v", record)
	}

	if !strings.HasSuffix(record["time"].(string), "Z") {
		t.Errorf("time must be UTC, got %v", record["time"])
	}
}

func TestLoggerHonoursLevel(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := telemetry.NewLogger(&buffer, slog.LevelWarn, "trip-service")

	logger.InfoContext(context.Background(), "hidden")

	if buffer.Len() != 0 {
		t.Errorf("info record must be dropped at warn level: %s", buffer.String())
	}
}
