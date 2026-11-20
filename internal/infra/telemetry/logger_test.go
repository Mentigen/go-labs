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

func TestLoggerTraceFields(t *testing.T) {
	ctx, span := sdktrace.NewTracerProvider().Tracer("test").Start(t.Context(), "operation")
	defer span.End()
	tests := []struct {
		name            string
		ctx             context.Context
		traceID, spanID string
	}{
		{name: "outside span", ctx: t.Context()},
		{
			name: "inside span", ctx: ctx,
			traceID: span.SpanContext().TraceID().String(),
			spanID:  span.SpanContext().SpanID().String(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buffer bytes.Buffer
			logger := telemetry.NewLogger(&buffer, slog.LevelInfo, "trip-service")

			logger.With("component", "test").InfoContext(tt.ctx, "trip completed")

			record := decode(t, &buffer)
			assert.Equal(t, tt.traceID, record["trace_id"])
			assert.Equal(t, tt.spanID, record["span_id"])
			assert.Equal(t, "trip-service", record["service"])
			assert.Equal(t, "test", record["component"])
			assert.Equal(t, "INFO", record["level"])
			assert.Equal(t, "trip completed", record["msg"])
			assert.True(t, strings.HasSuffix(record["time"].(string), "Z"))
		})
	}
}

func TestLoggerHonoursLevel(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := telemetry.NewLogger(&buffer, slog.LevelWarn, "trip-service")

	logger.InfoContext(context.Background(), "hidden")

	assert.Equal(t, 0, buffer.Len(), "info record must be dropped at warn level: %s", buffer.String())
}
