package trip_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if created.ID != tripID || created.UserID != userID || created.DriverID != driverID {
		t.Errorf("identifiers were not copied: %+v", created)
	}

	if created.Status != trip.StatusActive {
		t.Errorf("status = %q, want %q", created.Status, trip.StatusActive)
	}

	if created.FinishedAt != nil {
		t.Errorf("finished_at = %v, want nil", created.FinishedAt)
	}

	if !created.StartedAt.Equal(now) || !created.CreatedAt.Equal(now) || !created.UpdatedAt.Equal(now) {
		t.Errorf("timestamps must equal creation time: %+v", created)
	}
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
			if !errors.As(err, &validationError) {
				t.Fatalf("error = %v, want *ValidationError", err)
			}

			if validationError.Field != test.wantField {
				t.Errorf("field = %q, want %q", validationError.Field, test.wantField)
			}
		})
	}
}

func TestNewBoundaryValuesAreValid(t *testing.T) {
	t.Parallel()

	params := validParams()
	params.StartPoint = trip.Coordinates{Latitude: -90, Longitude: -180}
	params.EndPoint = trip.Coordinates{Latitude: 90, Longitude: 180}
	params.Price = 0

	if _, err := trip.New(tripID, now, params); err != nil {
		t.Fatalf("boundary values must be valid: %v", err)
	}
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
			if err := current.Finish(later); !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}

			if current.Status != trip.StatusCompleted {
				t.Errorf("status = %q, want %q", current.Status, trip.StatusCompleted)
			}

			if current.FinishedAt == nil || !current.FinishedAt.Equal(test.wantAt) {
				t.Errorf("finished_at = %v, want %v", current.FinishedAt, test.wantAt)
			}

			if !current.UpdatedAt.Equal(test.wantAt) {
				t.Errorf("updated_at = %v, want %v", current.UpdatedAt, test.wantAt)
			}
		})
	}
}

func TestValidationErrorMessage(t *testing.T) {
	t.Parallel()

	err := trip.NewValidationError("price", "must not be negative")

	if got, want := err.Error(), "invalid price: must not be negative"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}
