package model

import (
	"time"

	"github.com/google/uuid"
)

type StaleTrip struct {
	ID         uuid.UUID
	DriverID   uuid.UUID
	PositionAt time.Time
	UpdatedAt  time.Time
}

type PositionCursor struct {
	PositionAt time.Time
	ID         uuid.UUID
}

type StalePage struct {
	Trips  []StaleTrip
	Cutoff time.Time
}
