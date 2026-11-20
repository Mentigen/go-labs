package get_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/generated"
	"github.com/Mentigen/go-labs/internal/http/trip/get"
)

var tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func TestHandle(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	stored := tripdomain.Trip{ID: tripID, Status: tripdomain.StatusActive, Price: 700, LastPositionAt: &at}
	privateDetail := "pq: password authentication failed for user tripgo"
	returns := func(result tripdomain.Trip, err error) func(*gomock.Controller) *get.Handler {
		return func(ctrl *gomock.Controller) *get.Handler {
			usecase := NewMockusecase(ctrl)
			usecase.EXPECT().Execute(gomock.Any(), tripID).Return(result, err)
			return get.New(usecase, slog.New(slog.NewJSONHandler(io.Discard, nil)))
		}
	}
	tests := []struct {
		name     string
		setup    func(*gomock.Controller) *get.Handler
		in       uuid.UUID
		status   int
		want     generated.Trip
		wantCode string
	}{
		{
			name:  "trip with latest position",
			setup: returns(stored, nil), in: tripID, status: http.StatusOK,
			want: generated.Trip{Id: tripID, Status: generated.Active, Price: 700, LastPositionAt: &at},
		},
		{
			name:  "missing trip",
			setup: returns(tripdomain.Trip{}, tripdomain.ErrTripNotFound), in: tripID,
			status: http.StatusNotFound, wantCode: "trip_not_found",
		},
		{
			name:  "storage error",
			setup: returns(tripdomain.Trip{}, errors.New(privateDetail)), in: tripID,
			status: http.StatusInternalServerError, wantCode: "internal_error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.setup(gomock.NewController(t))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/trips/"+tt.in.String(), nil)
			rec := httptest.NewRecorder()
			handler.Handle(rec, req, tt.in)
			require.Equal(t, tt.status, rec.Code)
			if tt.wantCode != "" {
				var problem generated.Problem
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
				assert.Equal(t, tt.wantCode, problem.Code)
				assert.NotContains(t, rec.Body.String(), "password")
				assert.NotContains(t, rec.Body.String(), "tripgo\"")
				return
			}
			var got generated.Trip
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			assert.Equal(t, tt.want, got)
		})
	}
}
