package trip_test

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Mentigen/go-labs/internal/domain/trip"
)

var (
	now      = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	later    = now.Add(15 * time.Minute)
	tripID   = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	userID   = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	driverID = uuid.MustParse("33333333-3333-3333-3333-333333333333")
)

func validParams() trip.NewParams {
	return trip.NewParams{
		UserID:     userID,
		DriverID:   driverID,
		StartPoint: trip.Coordinates{Latitude: 55.75, Longitude: 37.61},
		EndPoint:   trip.Coordinates{Latitude: 55.80, Longitude: 37.70},
		Price:      500,
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	created, err := trip.New(tripID, now, validParams())
	require.NoError(t, err)

	assert.Equal(t, tripID, created.ID)
	assert.Equal(t, userID, created.UserID)
	assert.Equal(t, driverID, created.DriverID)

	assert.Equal(t, trip.StatusActive, created.Status)

	assert.Nil(t, created.FinishedAt, "finished_at = %v, want nil", created.FinishedAt)

	assert.True(t, created.StartedAt.Equal(now))
	assert.True(t, created.CreatedAt.Equal(now))
	assert.True(t, created.UpdatedAt.Equal(now))
}

func TestNewValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*trip.NewParams)
		wantField string
	}{
		{"nil user", func(p *trip.NewParams) { p.UserID = uuid.Nil }, "user_id"},
		{"nil driver", func(p *trip.NewParams) { p.DriverID = uuid.Nil }, "driver_id"},
		{"start latitude too high", func(p *trip.NewParams) { p.StartPoint.Latitude = 90.1 }, "start_point.latitude"},
		{"start latitude too low", func(p *trip.NewParams) { p.StartPoint.Latitude = -90.1 }, "start_point.latitude"},
		{"start longitude too high", func(p *trip.NewParams) { p.StartPoint.Longitude = 180.5 }, "start_point.longitude"},
		{"end latitude NaN", func(p *trip.NewParams) { p.EndPoint.Latitude = math.NaN() }, "end_point.latitude"},
		{"end longitude too low", func(p *trip.NewParams) { p.EndPoint.Longitude = -181 }, "end_point.longitude"},
		{"negative price", func(p *trip.NewParams) { p.Price = -1 }, "price"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			params := validParams()
			test.mutate(&params)

			_, err := trip.New(tripID, now, params)

			var validationError *trip.ValidationError
			require.ErrorAs(t, err, &validationError)

			assert.Equal(t, test.wantField, validationError.Field)
		})
	}
}

func TestNewBoundaryValuesAreValid(t *testing.T) {
	t.Parallel()

	params := validParams()
	params.StartPoint = trip.Coordinates{Latitude: -90, Longitude: -180}
	params.EndPoint = trip.Coordinates{Latitude: 90, Longitude: 180}
	params.Price = 0

	_, err := trip.New(tripID, now, params)
	require.NoError(t, err)
}

func TestFinish(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current trip.Trip
		wantErr error
		wantAt  time.Time
	}{
		{
			name:    "active trip becomes completed",
			current: trip.Trip{ID: tripID, Status: trip.StatusActive, UpdatedAt: now},
			wantAt:  later,
		},
		{
			name:    "completed trip cannot be finished again",
			current: trip.Trip{ID: tripID, Status: trip.StatusCompleted, FinishedAt: &now, UpdatedAt: now},
			wantErr: trip.ErrTripCompleted,
			wantAt:  now,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			current := test.current
			err := current.Finish(later)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, trip.StatusCompleted, current.Status)

			require.NotNil(t, current.FinishedAt)
			assert.True(t, current.FinishedAt.Equal(test.wantAt))

			assert.True(t, current.UpdatedAt.Equal(test.wantAt), "updated_at = %v, want %v", current.UpdatedAt, test.wantAt)
		})
	}
}

func TestValidationErrorMessage(t *testing.T) {
	t.Parallel()

	err := trip.NewValidationError("price", "must not be negative")

	assert.EqualError(t, err, "invalid price: must not be negative")
}
