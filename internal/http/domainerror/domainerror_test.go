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

			if status != test.wantStatus || int(body.Status) != test.wantStatus {
				t.Errorf("status = %d (body %d), want %d", status, body.Status, test.wantStatus)
			}

			if body.Code != test.wantCode {
				t.Errorf("code = %q, want %q", body.Code, test.wantCode)
			}

			if want := "https://tripgo.example/problems/" + test.wantType; body.Type != want {
				t.Errorf("type = %q, want %q", body.Type, want)
			}

			if body.Title == "" || body.Detail == nil || *body.Detail == "" {
				t.Errorf("title and detail must be filled: %+v", body)
			}
		})
	}
}

func TestProblemKeepsValidationDetails(t *testing.T) {
	t.Parallel()

	_, body := domainerror.Problem(trip.NewValidationError("start_point.latitude", "must be between -90 and 90"))

	if body.Detail == nil || !strings.Contains(*body.Detail, "start_point.latitude") {
		t.Errorf("detail must name the failing field, got %v", body.Detail)
	}
}

func TestProblemHidesInternalDetails(t *testing.T) {
	t.Parallel()

	secret := "ERROR: relation \"trips\" does not exist (SQLSTATE 42P01) password=hunter2"

	_, body := domainerror.Problem(errors.New(secret))

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal problem: %v", err)
	}

	for _, leaked := range []string{"SQLSTATE", "relation", "hunter2", "trips"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("problem leaks %q: %s", leaked, raw)
		}
	}
}

func TestWriteSetsHeadersInstanceAndLogsServerErrorsOnce(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/trips/abc", nil)
	recorder := httptest.NewRecorder()

	domainerror.Write(recorder, request, logger, errors.New("database exploded"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("content type = %q", got)
	}

	var body generated.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Instance == nil || *body.Instance != "/api/v1/trips/abc" {
		t.Errorf("instance = %v", body.Instance)
	}

	if strings.Contains(recorder.Body.String(), "database exploded") {
		t.Errorf("response leaks the internal error: %s", recorder.Body.String())
	}

	if got := strings.Count(logs.String(), "database exploded"); got != 1 {
		t.Errorf("internal error logged %d times, want 1", got)
	}
}

func TestWriteDoesNotLogClientErrors(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/trips/abc", nil)
	recorder := httptest.NewRecorder()

	domainerror.Write(recorder, request, logger, trip.ErrTripNotFound)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}

	if logs.Len() != 0 {
		t.Errorf("client errors must not be logged by the mapper: %s", logs.String())
	}
}
