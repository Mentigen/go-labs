package trip

import (
	"time"

	"github.com/google/uuid"
)

type PositionInput struct {
	Coordinates Coordinates
	RecordedAt  time.Time
}

type Position struct {
	ID          int64
	TripID      uuid.UUID
	Coordinates Coordinates
	RecordedAt  time.Time
}

func NewPosition(tripID uuid.UUID, input PositionInput, now time.Time) (Position, error) {
	if err := validateStruct.Struct(input.Coordinates); err != nil {
		return Position{}, NewValidationError("coordinates", "latitude must be between -90 and 90, longitude between -180 and 180")
	}
	if input.RecordedAt.IsZero() || input.RecordedAt.After(now.Add(time.Minute)) {
		return Position{}, NewValidationError("recorded_at", "must be set and no more than one minute in the future")
	}
	return Position{TripID: tripID, Coordinates: input.Coordinates, RecordedAt: input.RecordedAt.UTC()}, nil
}
