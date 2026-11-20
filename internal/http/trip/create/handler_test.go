package create_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/generated"
	"github.com/Mentigen/go-labs/internal/http/trip/create"
	"github.com/Mentigen/go-labs/internal/model"
)

var (
	tripID   = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	userID   = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	driverID = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	key      = uuid.MustParse("44444444-4444-4444-4444-444444444444")
)

const validBody = `{
	"user_id": "22222222-2222-2222-2222-222222222222",
	"driver_id": "33333333-3333-3333-3333-333333333333",
	"start_point": {"latitude": 55.75, "longitude": 37.61},
	"end_point": {"latitude": 55.8, "longitude": 37.7},
	"price": 500
}`

func newHandler(t *testing.T) (*create.Handler, *Mockusecase) {
	t.Helper()

	usecase := NewMockusecase(gomock.NewController(t))
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	return create.New(usecase, logger), usecase
}

func serve(handler *create.Handler, body string, params generated.CreateTripParams) *httptest.ResponseRecorder {
	request := httptest.NewRequestWithContext(
		context.Background(), http.MethodPost, "/api/v1/trips", strings.NewReader(body),
	)
	recorder := httptest.NewRecorder()

	handler.Handle(recorder, request, params)

	return recorder
}

func decodeProblem(t *testing.T, recorder *httptest.ResponseRecorder) generated.Problem {
	t.Helper()

	var problem generated.Problem
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &problem))

	return problem
}

func createdTrip() tripdomain.Trip {
	return tripdomain.Trip{
		ID:       tripID,
		UserID:   userID,
		DriverID: driverID,
		Price:    500,
		Status:   tripdomain.StatusActive,
	}
}

func TestHandleCreateAndReplay(t *testing.T) {
	responseTrip := createdTrip()
	tests := []struct {
		name     string
		key      *uuid.UUID
		created  bool
		status   int
		location string
	}{
		{"create", nil, true, http.StatusCreated, "/api/v1/trips/" + tripID.String()},
		{"replay", &key, false, http.StatusOK, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, usecase := newHandler(t)
			input := model.CreateTripInput{
				UserID: userID, DriverID: driverID, Price: 500,
				StartPoint:     tripdomain.Coordinates{Latitude: 55.75, Longitude: 37.61},
				EndPoint:       tripdomain.Coordinates{Latitude: 55.8, Longitude: 37.7},
				IdempotencyKey: tt.key,
			}
			usecase.EXPECT().Execute(gomock.Any(), input).Return(model.CreateTripResult{Trip: responseTrip, Created: tt.created}, nil)

			rec := serve(handler, validBody, generated.CreateTripParams{IdempotencyKey: tt.key})
			require.Equal(t, tt.status, rec.Code)
			assert.Equal(t, tt.location, rec.Header().Get("Location"))
			var body generated.Trip
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tripID, body.Id)
			assert.Equal(t, generated.Active, body.Status)
		})
	}
}

func TestHandleMapsUsecaseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"driver busy", tripdomain.ErrDriverBusy, http.StatusConflict, "driver_busy"},
		{"idempotency conflict", tripdomain.ErrIdempotencyConflict, http.StatusConflict, "idempotency_conflict"},
		{"domain validation", tripdomain.NewValidationError("price", "must not be negative"), http.StatusBadRequest, "invalid_request"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, usecase := newHandler(t)

			usecase.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(model.CreateTripResult{}, test.err)

			recorder := serve(handler, validBody, generated.CreateTripParams{})

			require.Equal(t, test.wantStatus, recorder.Code)

			assert.Equal(t, test.wantCode, decodeProblem(t, recorder).Code)
		})
	}
}

func TestHandleRejectsMalformedBody(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"not json":       `{"user_id":`,
		"empty body":     ``,
		"wrong type":     `{"price": "free"}`,
		"trailing value": validBody + validBody,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newHandler(t)

			recorder := serve(handler, body, generated.CreateTripParams{})

			require.Equal(t, http.StatusBadRequest, recorder.Code)

			assert.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"))

			assert.Equal(t, "invalid_request", decodeProblem(t, recorder).Code)
		})
	}
}

func TestHandleHidesRepositoryErrorDetails(t *testing.T) {
	t.Parallel()

	handler, usecase := newHandler(t)

	secret := "ERROR: connection to 10.0.0.5:5432 refused password=hunter2 SQLSTATE 08006"

	usecase.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(model.CreateTripResult{}, errors.New(secret))

	recorder := serve(handler, validBody, generated.CreateTripParams{})

	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	for _, leaked := range []string{"hunter2", "10.0.0.5", "SQLSTATE", "connection"} {
		assert.NotContains(t, recorder.Body.String(), leaked, "response leaks %q: %s", leaked, recorder.Body.String())
	}

	assert.Equal(t, "internal_error", decodeProblem(t, recorder).Code)
}
