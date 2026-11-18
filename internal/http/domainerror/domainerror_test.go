package domainerror_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/generated"
	"github.com/Mentigen/go-labs/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantType   string
	}{
		{"validation error", trip.NewValidationError("price", "must not be negative"), 400, "invalid_request", "invalid-request"},
		{"request error", domainerror.NewRequestError("invalid JSON body"), 400, "invalid_request", "invalid-request"},
		{"route not found", domainerror.ErrRouteNotFound, 404, "not_found", "not-found"},
		{"method not allowed", domainerror.ErrMethodNotAllowed, 405, "method_not_allowed", "method-not-allowed"},
		{"trip not found", trip.ErrTripNotFound, 404, "trip_not_found", "trip-not-found"},
		{"trip completed", trip.ErrTripCompleted, 409, "trip_completed", "trip-completed"},
		{"driver busy", trip.ErrDriverBusy, 409, "driver_busy", "driver-busy"},
		{"idempotency conflict", trip.ErrIdempotencyConflict, 409, "idempotency_conflict", "idempotency-conflict"},
		{"wrapped domain error", fmt.Errorf("finish trip: %w", trip.ErrTripNotFound), 404, "trip_not_found", "trip-not-found"},
		{"joined domain error", errors.Join(errors.New("rollback failed"), trip.ErrDriverBusy), 409, "driver_busy", "driver-busy"},
		{"unknown error", errors.New("boom"), 500, "internal_error", "internal-error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			status, body := domainerror.Problem(test.err)

			assert.Equal(t, test.wantStatus, status)
			assert.Equal(t, test.wantStatus, int(body.Status))

			assert.Equal(t, test.wantCode, body.Code)

			want := "https://tripgo.example/problems/" + test.wantType
			assert.Equal(t, want, body.Type)

			assert.NotEqual(t, "", body.Title)
			require.NotNil(t, body.Detail)
			assert.NotEqual(t, "", *body.Detail)
		})
	}
}

func TestProblemKeepsValidationDetails(t *testing.T) {
	t.Parallel()

	_, body := domainerror.Problem(trip.NewValidationError("start_point.latitude", "must be between -90 and 90"))

	require.NotNil(t, body.Detail)
	assert.Contains(t, *body.Detail, "start_point.latitude")
}

func TestProblemHidesInternalDetails(t *testing.T) {
	t.Parallel()

	secret := "ERROR: relation \"trips\" does not exist (SQLSTATE 42P01) password=hunter2"

	_, body := domainerror.Problem(errors.New(secret))

	raw, err := json.Marshal(body)
	require.NoError(t, err, "marshal problem: %v", err)

	for _, leaked := range []string{"SQLSTATE", "relation", "hunter2", "trips"} {
		assert.NotContains(t, string(raw), leaked, "problem leaks %q: %s", leaked, raw)
	}
}

func TestWriteSetsHeadersInstanceAndLogsServerErrorsOnce(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/trips/abc", nil)
	recorder := httptest.NewRecorder()

	domainerror.Write(recorder, request, logger, errors.New("database exploded"))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	assert.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"))

	var body generated.Problem
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))

	require.NotNil(t, body.Instance)
	assert.Equal(t, "/api/v1/trips/abc", *body.Instance)

	assert.NotContains(t, recorder.Body.String(), "database exploded", "response leaks the internal error: %s", recorder.Body.String())

	assert.EqualValues(t, 1, strings.Count(logs.String(), "database exploded"))
}

func TestWriteDoesNotLogClientErrors(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/trips/abc", nil)
	recorder := httptest.NewRecorder()

	domainerror.Write(recorder, request, logger, trip.ErrTripNotFound)

	require.Equal(t, http.StatusNotFound, recorder.Code)

	assert.Equal(t, 0, logs.Len(), "client errors must not be logged by the mapper: %s", logs.String())
}
