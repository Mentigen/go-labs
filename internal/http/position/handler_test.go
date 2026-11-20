package position_test

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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	"github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/generated"
	httpserver "github.com/Mentigen/go-labs/internal/http"
	"github.com/Mentigen/go-labs/internal/http/position"
)

var tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

const validBody = `{"coordinates":{"latitude":37.3,"longitude":-120.4},"recorded_at":"2026-11-01T12:00:00Z"}`

var positionPath = "/api/v1/trips/" + tripID.String() + "/positions"

func router(t *testing.T, ctrl *gomock.Controller) (http.Handler, *Mocksaver, *Mocklister) {
	t.Helper()
	save, list := NewMocksaver(ctrl), NewMocklister(ctrl)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler, err := httpserver.NewRouter(httpserver.Options{
		Handlers:       httpserver.Handlers{Positions: position.New(save, list, logger)},
		Logger:         logger,
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  metricnoop.NewMeterProvider(),
		Propagator:     propagation.TraceContext{},
	})
	require.NoError(t, err)
	return handler, save, list
}

func post(handler http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, positionPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	require.Equal(t, status, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	var problem generated.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, code, problem.Code)
}

func TestSavePosition(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	point := trip.Coordinates{Latitude: 37.3, Longitude: -120.4}
	input := trip.PositionInput{Coordinates: point, RecordedAt: at}
	saved := trip.Position{ID: 42, TripID: tripID, Coordinates: point, RecordedAt: at}
	returns := func(position trip.Position, err error) func(*Mocksaver) {
		return func(save *Mocksaver) { save.EXPECT().Execute(gomock.Any(), tripID, input).Return(position, err) }
	}

	type scenario struct {
		name     string
		setup    func(*Mocksaver)
		in       string
		status   int
		want     generated.TripPosition
		wantCode string
	}
	invalid := func(name, body string) scenario {
		return scenario{name: name, in: body, status: http.StatusBadRequest, wantCode: "invalid_request"}
	}
	tests := []scenario{
		{
			name:   "position saved",
			setup:  returns(saved, nil),
			in:     validBody,
			status: http.StatusCreated,
			want: generated.TripPosition{
				Id: 42, TripId: tripID, RecordedAt: at,
				Coordinates: generated.Coordinates{Latitude: 37.3, Longitude: -120.4},
			},
		},
		{name: "missing trip", setup: returns(trip.Position{}, trip.ErrTripNotFound), in: validBody, status: http.StatusNotFound, wantCode: "trip_not_found"},
		{name: "completed trip", setup: returns(trip.Position{}, trip.ErrTripCompleted), in: validBody, status: http.StatusConflict, wantCode: "trip_completed"},
		{name: "future position", setup: returns(trip.Position{}, trip.NewValidationError("recorded_at", "future")), in: validBody, status: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "storage failure", setup: returns(trip.Position{}, errors.New("storage failure")), in: validBody, status: http.StatusInternalServerError, wantCode: "internal_error"},
		invalid("two objects", validBody+" {}"),
		invalid("null body", `null`),
		invalid("missing fields", `{}`),
		invalid("null coordinates", `{"coordinates":null,"recorded_at":"2026-11-01T12:00:00Z"}`),
		invalid("missing coordinates", `{"coordinates":{},"recorded_at":"2026-11-01T12:00:00Z"}`),
		invalid("null latitude", `{"coordinates":{"latitude":null,"longitude":0},"recorded_at":"2026-11-01T12:00:00Z"}`),
		invalid("null time", `{"coordinates":{"latitude":0,"longitude":0},"recorded_at":null}`),
		invalid("invalid time", `{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"invalid"}`),
		invalid("latitude out of range", `{"coordinates":{"latitude":91,"longitude":0},"recorded_at":"2026-11-01T12:00:00Z"}`),
		invalid("unknown field", `{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"2026-11-01T12:00:00Z","extra":1}`),
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, save, _ := router(t, gomock.NewController(t))
			if tt.setup != nil {
				tt.setup(save)
			}
			rec := post(h, tt.in)
			if tt.wantCode != "" {
				assertProblem(t, rec, tt.status, tt.wantCode)
				return
			}
			require.Equal(t, tt.status, rec.Code)
			var body generated.TripPosition
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tt.want, body)
		})
	}
}

func TestListPositions(t *testing.T) {
	t.Parallel()
	returns := func(positions []trip.Position, err error) func(*Mocklister) {
		return func(list *Mocklister) { list.EXPECT().Execute(gomock.Any(), tripID).Return(positions, err) }
	}
	tests := []struct {
		name     string
		setup    func(*Mocklister)
		status   int
		want     []int64
		wantCode string
	}{
		{name: "empty route", setup: returns(nil, nil), status: http.StatusOK, want: []int64{}},
		{name: "ordered route", setup: returns([]trip.Position{{ID: 3}, {ID: 2}}, nil), status: http.StatusOK, want: []int64{3, 2}},
		{name: "missing trip", setup: returns(nil, trip.ErrTripNotFound), status: http.StatusNotFound, wantCode: "trip_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, list := router(t, gomock.NewController(t))
			tt.setup(list)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, positionPath, nil))
			if tt.wantCode != "" {
				assertProblem(t, rec, tt.status, tt.wantCode)
				return
			}
			require.Equal(t, tt.status, rec.Code)
			var body generated.TripPositionsResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.NotNil(t, body.Positions)
			ids := make([]int64, 0, len(body.Positions))
			for _, pos := range body.Positions {
				ids = append(ids, pos.Id)
			}
			assert.Equal(t, tt.want, ids)
		})
	}
}
