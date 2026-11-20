package model

import (
	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/domain/trip"
)

type CreateTripInput struct {
	UserID   uuid.UUID
	DriverID uuid.UUID

	StartPoint trip.Coordinates
	EndPoint   trip.Coordinates

	Price int64

	IdempotencyKey *uuid.UUID
}

type CreateTripResult struct {
	Trip    trip.Trip
	Created bool
}

type IdempotencyRecord struct {
	RequestHash string
	TripID      uuid.UUID
}
