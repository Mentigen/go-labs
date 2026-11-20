package push_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/Mentigen/go-labs/internal/gateway/push"
	pushapi "github.com/Mentigen/go-labs/internal/generated/push"
)

func verifyCommand(t *testing.T, r *http.Request, requestID, tripID, driverID uuid.UUID) {
	t.Helper()
	assert.Equal(t, http.MethodPost, r.Method)
	assert.Equal(t, "/api/v1/push", r.URL.Path)
	assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
	assert.NotEmpty(t, r.Header.Get("traceparent"))

	var body pushapi.PushRequest
	if assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
		assert.Equal(t, requestID.String(), body.RequestId)
		assert.Equal(t, driverID, body.RecipientId)
		assert.Equal(t, pushapi.REQUESTPOSITION, body.Kind)
		assert.Equal(t, &map[string]string{"trip_id": tripID.String()}, body.Data)
	}
}

func TestRequestPosition(t *testing.T) {
	t.Parallel()
	requestID, driverID, tripID := uuid.New(), uuid.New(), uuid.New()
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})
	type responder func(*testing.T, http.ResponseWriter, *http.Request)
	reply := func(status int) responder {
		return func(_ *testing.T, w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }
	}
	accepted := func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		verifyCommand(t, r, requestID, tripID, driverID)
		w.WriteHeader(http.StatusAccepted)
	}
	truncated := func(_ *testing.T, w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	blocked := func(_ *testing.T, w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusAccepted)
		}
	}
	deadline := func(t assert.TestingT, err error, _ ...any) bool {
		var statusError *push.HTTPError
		return assert.ErrorIs(t, err, context.DeadlineExceeded) && assert.NotErrorAs(t, err, &statusError)
	}
	tests := []struct {
		name    string
		setup   responder
		timeout time.Duration
		wantErr assert.ErrorAssertionFunc
	}{
		{"push accepted", accepted, time.Second, assert.NoError},
		{"invalid command", reply(http.StatusBadRequest), time.Second, httpError(http.StatusBadRequest)},
		{"rate limited", reply(http.StatusTooManyRequests), time.Second, httpError(http.StatusTooManyRequests)},
		{"unavailable", reply(http.StatusServiceUnavailable), time.Second, httpError(http.StatusServiceUnavailable)},
		{"redirect", reply(http.StatusFound), time.Second, httpError(http.StatusFound)},
		{"unavailable with truncated body", truncated, time.Second, httpError(http.StatusServiceUnavailable)},
		{"request timed out", blocked, 20 * time.Millisecond, deadline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { tt.setup(t, w, r) }))
			t.Cleanup(server.Close)

			gateway, err := push.New(server.URL, tt.timeout, noop.NewTracerProvider(), propagation.TraceContext{}, func() uuid.UUID { return requestID })
			require.NoError(t, err)
			t.Cleanup(gateway.Close)
			ctx := trace.ContextWithSpanContext(t.Context(), spanContext)
			err = gateway.RequestPosition(ctx, tripID, driverID)
			tt.wantErr(t, err)
		})
	}
}

func httpError(status int) assert.ErrorAssertionFunc {
	return func(t assert.TestingT, err error, _ ...any) bool {
		var got *push.HTTPError
		return assert.ErrorAs(t, err, &got) && assert.Equal(t, status, got.Status)
	}
}
