package trip

import (
	"errors"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validateStruct = validator.New()

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Coordinates struct {
	Latitude  float64 `validate:"gte=-90,lte=90"`
	Longitude float64 `validate:"gte=-180,lte=180"`
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
	UserID   uuid.UUID `validate:"required"`
	DriverID uuid.UUID `validate:"required"`

	StartPoint Coordinates
	EndPoint   Coordinates

	Price int64 `validate:"gte=0"`
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
	err := validateStruct.Struct(params)
	if err == nil {
		return nil
	}
	var violations validator.ValidationErrors
	if !errors.As(err, &violations) {
		return err
	}
	violation := violations[0]
	field := strings.TrimPrefix(violation.StructNamespace(), "NewParams.")
	names := strings.NewReplacer("UserID", "user_id", "DriverID", "driver_id", "StartPoint", "start_point", "EndPoint", "end_point", "Latitude", "latitude", "Longitude", "longitude", "Price", "price")
	reason := "must not be empty"
	switch violation.StructField() {
	case "Latitude":
		reason = "must be between -90 and 90"
	case "Longitude":
		reason = "must be between -180 and 180"
	case "Price":
		reason = "must not be negative"
	}
	return NewValidationError(names.Replace(field), reason)
}
