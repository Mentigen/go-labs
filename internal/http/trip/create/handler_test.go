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
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v (%s)", err, recorder.Body.String())
	}

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

func TestHandleCreatesTrip(t *testing.T) {
	t.Parallel()

	handler, usecase := newHandler(t)

	usecase.EXPECT().
		Execute(gomock.Any(), gomock.Cond(func(input model.CreateTripInput) bool {
			return input.UserID == userID && input.DriverID == driverID && input.Price == 500 &&
				input.StartPoint.Latitude == 55.75 && input.EndPoint.Longitude == 37.7 &&
				input.IdempotencyKey == nil
		})).
		Return(model.CreateTripResult{Trip: createdTrip(), Created: true}, nil)

	recorder := serve(handler, validBody, generated.CreateTripParams{})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", recorder.Code, recorder.Body.String())
	}

	if got, want := recorder.Header().Get("Location"), "/api/v1/trips/"+tripID.String(); got != want {
		t.Errorf("location = %q, want %q", got, want)
	}

	var body generated.Trip
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Id != tripID || body.Status != generated.TripStatus(tripdomain.StatusActive) {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestHandlePassesIdempotencyKeyAndAnswers200OnReplay(t *testing.T) {
	t.Parallel()

	handler, usecase := newHandler(t)

	usecase.EXPECT().
		Execute(gomock.Any(), gomock.Cond(func(input model.CreateTripInput) bool {
			return input.IdempotencyKey != nil && *input.IdempotencyKey == key
		})).
		Return(model.CreateTripResult{Trip: createdTrip(), Created: false}, nil)

	recorder := serve(handler, validBody, generated.CreateTripParams{IdempotencyKey: &key})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	if location := recorder.Header().Get("Location"); location != "" {
		t.Errorf("replay must not set Location, got %q", location)
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

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}

			if problem := decodeProblem(t, recorder); problem.Code != test.wantCode {
				t.Errorf("code = %q, want %q", problem.Code, test.wantCode)
			}
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

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", recorder.Code)
			}

			if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("content type = %q", got)
			}

			if problem := decodeProblem(t, recorder); problem.Code != "invalid_request" {
				t.Errorf("code = %q, want invalid_request", problem.Code)
			}
		})
	}
}

func TestHandleHidesRepositoryErrorDetails(t *testing.T) {
	t.Parallel()

	handler, usecase := newHandler(t)

	secret := "ERROR: connection to 10.0.0.5:5432 refused password=hunter2 SQLSTATE 08006"

	usecase.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(model.CreateTripResult{}, errors.New(secret))

	recorder := serve(handler, validBody, generated.CreateTripParams{})

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}

	for _, leaked := range []string{"hunter2", "10.0.0.5", "SQLSTATE", "connection"} {
		if strings.Contains(recorder.Body.String(), leaked) {
			t.Errorf("response leaks %q: %s", leaked, recorder.Body.String())
		}
	}

	if problem := decodeProblem(t, recorder); problem.Code != "internal_error" {
		t.Errorf("code = %q, want internal_error", problem.Code)
	}
}
