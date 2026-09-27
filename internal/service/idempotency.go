package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/model"
)

const idempotencyKeyTTL = 24 * time.Hour

var errIdempotencyKeyLost = errors.New("idempotency key claimed by a concurrent request")

type idempotencyPayload struct {
	UserID     uuid.UUID         `json:"user_id"`
	DriverID   uuid.UUID         `json:"driver_id"`
	StartPoint model.Coordinates `json:"start_point"`
	EndPoint   model.Coordinates `json:"end_point"`
	Price      int64             `json:"price"`
}

func hashCreateTripInput(input CreateTripInput) string {
	payload := idempotencyPayload{
		UserID:     input.UserID,
		DriverID:   input.DriverID,
		StartPoint: input.StartPoint,
		EndPoint:   input.EndPoint,
		Price:      input.Price,
	}

	raw, _ := json.Marshal(payload)

	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:])
}

func (s *TripService) CreateIdempotent(
	ctx context.Context,
	input CreateTripInput,
	key uuid.UUID,
) (trip model.Trip, created bool, err error) {
	requestHash := hashCreateTripInput(input)

	if existing, found, err := s.lookupIdempotent(ctx, key, requestHash); err != nil {
		return model.Trip{}, false, err
	} else if found {
		return existing, false, nil
	}

	trip = newTrip(input)

	err = s.txManager.Do(ctx, func(ctx context.Context) error {
		claimed, err := s.repository.ClaimIdempotencyKey(ctx, key, requestHash, trip.ID)
		if err != nil {
			return err
		}

		if !claimed {
			return errIdempotencyKeyLost
		}

		if err := s.repository.Create(ctx, trip); err != nil {
			return err
		}

		return s.repository.AddCreatedHistory(ctx, trip.ID)
	})

	if errors.Is(err, errIdempotencyKeyLost) {
		existing, found, lookupErr := s.lookupIdempotent(ctx, key, requestHash)
		if lookupErr != nil {
			return model.Trip{}, false, lookupErr
		}

		if !found {
			return model.Trip{}, false, fmt.Errorf("idempotency key disappeared after conflict")
		}

		return existing, false, nil
	}

	if err != nil {
		return model.Trip{}, false, err
	}

	return trip, true, nil
}

func (s *TripService) lookupIdempotent(
	ctx context.Context,
	key uuid.UUID,
	requestHash string,
) (model.Trip, bool, error) {
	record, found, err := s.repository.GetIdempotencyKey(ctx, key, idempotencyKeyTTL)
	if err != nil {
		return model.Trip{}, false, err
	}

	if !found {
		return model.Trip{}, false, nil
	}

	if record.RequestHash != requestHash {
		return model.Trip{}, false, model.ErrIdempotencyConflict
	}

	trip, err := s.repository.GetByID(ctx, record.TripID)
	if err != nil {
		return model.Trip{}, false, err
	}

	return trip, true, nil
}
