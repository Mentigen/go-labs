package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Mentigen/go-labs/internal/infra/telemetry"
)

func decode(t *testing.T, buffer *bytes.Buffer) map[string]any {
	t.Helper()

	record := map[string]any{}
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buffer.Bytes()), &record))

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

	assert.Equal(t, span.SpanContext().TraceID().String(), record["trace_id"])

	assert.Equal(t, span.SpanContext().SpanID().String(), record["span_id"])
}

func TestLoggerWritesRequiredFieldsWithoutSpan(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := telemetry.NewLogger(&buffer, slog.LevelInfo, "trip-service")

	logger.With("component", "test").InfoContext(context.Background(), "outside span")

	record := decode(t, &buffer)

	for _, field := range []string{"time", "level", "msg", "service"} {
		assert.Contains(t, record, field)
	}

	assert.Equal(t, "trip-service", record["service"])
	assert.Equal(t, "test", record["component"])

	assert.NotContains(t, record, "trace_id")

	assert.True(t, strings.HasSuffix(record["time"].(string), "Z"))
}

func TestLoggerHonoursLevel(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := telemetry.NewLogger(&buffer, slog.LevelWarn, "trip-service")

	logger.InfoContext(context.Background(), "hidden")

	assert.Equal(t, 0, buffer.Len(), "info record must be dropped at warn level: %s", buffer.String())
}
