package finish_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/generated"
	"github.com/Mentigen/go-labs/internal/http/trip/finish"
)

var tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func serve(t *testing.T, result tripdomain.Trip, err error) *httptest.ResponseRecorder {
	t.Helper()

	usecase := NewMockusecase(gomock.NewController(t))
	usecase.EXPECT().Execute(gomock.Any(), tripID).Return(result, err)

	handler := finish.New(usecase, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/trips/"+tripID.String()+"/finish", nil)
	recorder := httptest.NewRecorder()

	handler.Handle(recorder, request, tripID)

	return recorder
}

func TestHandleReturnsTrip(t *testing.T) {
	t.Parallel()

	recorder := serve(t, tripdomain.Trip{ID: tripID, Status: tripdomain.StatusCompleted, Price: 700}, nil)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body generated.Trip
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))

	assert.Equal(t, generated.Completed, body.Status)
	assert.Equal(t, tripID, body.Id)
	assert.EqualValues(t, 700, body.Price)
}

func TestHandleErrors(t *testing.T) {
	storageError := errors.New("deadlock on private database host")
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"missing trip", tripdomain.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
		{"already completed", tripdomain.ErrTripCompleted, http.StatusConflict, "trip_completed"},
		{"storage failure", storageError, http.StatusInternalServerError, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, tripdomain.Trip{}, tt.err)
			require.Equal(t, tt.status, rec.Code)
			var problem generated.Problem
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			assert.Equal(t, tt.code, problem.Code)
			assert.NotContains(t, rec.Body.String(), "private database host")
		})
	}
}
