package get_test

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
	"github.com/Mentigen/go-labs/internal/http/trip/get"
)

var tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func serve(t *testing.T, result tripdomain.Trip, err error) *httptest.ResponseRecorder {
	t.Helper()

	usecase := NewMockusecase(gomock.NewController(t))
	usecase.EXPECT().Execute(gomock.Any(), tripID).Return(result, err)

	handler := get.New(usecase, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/trips/"+tripID.String(), nil)
	recorder := httptest.NewRecorder()

	handler.Handle(recorder, request, tripID)

	return recorder
}

func TestHandleReturnsTrip(t *testing.T) {
	t.Parallel()

	recorder := serve(t, tripdomain.Trip{ID: tripID, Status: tripdomain.StatusActive, Price: 700}, nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	var body generated.Trip
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Id != tripID || body.Price != 700 {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestHandleTripNotFound(t *testing.T) {
	t.Parallel()

	recorder := serve(t, tripdomain.Trip{}, tripdomain.ErrTripNotFound)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}

	var problem generated.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}

	if problem.Code != "trip_not_found" {
		t.Errorf("code = %q, want trip_not_found", problem.Code)
	}
}

func TestHandleHidesInternalErrorDetails(t *testing.T) {
	t.Parallel()

	recorder := serve(t, tripdomain.Trip{}, errors.New("pq: password authentication failed for user tripgo"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}

	if strings.Contains(recorder.Body.String(), "password") || strings.Contains(recorder.Body.String(), "tripgo\"") {
		t.Errorf("response leaks internal details: %s", recorder.Body.String())
	}
}
