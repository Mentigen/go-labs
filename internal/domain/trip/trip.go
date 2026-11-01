package trip

import (
	"time"

	"github.com/google/uuid"
)

const (
	minLatitude  = -90
	maxLatitude  = 90
	minLongitude = -180
	maxLongitude = 180
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type Trip struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	DriverID uuid.UUID

	StartPoint Coordinates
	EndPoint   Coordinates

	Price  int64
	Status Status

	StartedAt  time.Time
	FinishedAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

type NewParams struct {
	UserID   uuid.UUID
	DriverID uuid.UUID

	StartPoint Coordinates
	EndPoint   Coordinates

	Price int64
}

func New(id uuid.UUID, now time.Time, params NewParams) (Trip, error) {
	if err := validate(params); err != nil {
		return Trip{}, err
	}

	return Trip{
		ID:       id,
		UserID:   params.UserID,
		DriverID: params.DriverID,

		StartPoint: params.StartPoint,
		EndPoint:   params.EndPoint,

		Price:  params.Price,
		Status: StatusActive,

		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (t *Trip) Finish(at time.Time) error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}

	t.Status = StatusCompleted
	t.FinishedAt = &at
	t.UpdatedAt = at

	return nil
}

func validate(params NewParams) error {
	if params.UserID == uuid.Nil {
		return NewValidationError("user_id", "must not be empty")
	}

	if params.DriverID == uuid.Nil {
		return NewValidationError("driver_id", "must not be empty")
	}

	if err := validateCoordinates("start_point", params.StartPoint); err != nil {
		return err
	}

	if err := validateCoordinates("end_point", params.EndPoint); err != nil {
		return err
	}

	if params.Price < 0 {
		return NewValidationError("price", "must not be negative")
	}

	return nil
}

func validateCoordinates(field string, point Coordinates) error {
	if !(point.Latitude >= minLatitude && point.Latitude <= maxLatitude) {
		return NewValidationError(field+".latitude", "must be between -90 and 90")
	}

	if !(point.Longitude >= minLongitude && point.Longitude <= maxLongitude) {
		return NewValidationError(field+".longitude", "must be between -180 and 180")
	}

	return nil
}
