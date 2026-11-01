package create

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/model"
)

const idempotencyKeyTTL = 24 * time.Hour

var errIdempotencyKeyLost = errors.New("idempotency key claimed by a concurrent request")

type Usecase struct {
	repository tripRepository
	txManager  txManager
	metrics    metrics
	logger     *slog.Logger
	now        func() time.Time
	newID      func() uuid.UUID
}

func New(
	repository tripRepository,
	txManager txManager,
	metrics metrics,
	logger *slog.Logger,
	now func() time.Time,
	newID func() uuid.UUID,
) *Usecase {
	return &Usecase{
		repository: repository,
		txManager:  txManager,
		metrics:    metrics,
		logger:     logger,
		now:        now,
		newID:      newID,
	}
}

func (u *Usecase) Execute(ctx context.Context, input model.CreateTripInput) (model.CreateTripResult, error) {
	params := tripdomain.NewParams{
		UserID:     input.UserID,
		DriverID:   input.DriverID,
		StartPoint: input.StartPoint,
		EndPoint:   input.EndPoint,
		Price:      input.Price,
	}

	trip, err := tripdomain.New(u.newID(), u.now().UTC(), params)
	if err != nil {
		return model.CreateTripResult{}, err
	}

	if input.IdempotencyKey == nil {
		err = u.txManager.Do(ctx, func(ctx context.Context) error {
			return u.insert(ctx, trip)
		})
		if err != nil {
			return model.CreateTripResult{}, err
		}

		u.created(ctx, trip)

		return model.CreateTripResult{Trip: trip, Created: true}, nil
	}

	return u.executeIdempotent(ctx, trip, params, *input.IdempotencyKey)
}

func (u *Usecase) executeIdempotent(
	ctx context.Context,
	trip tripdomain.Trip,
	params tripdomain.NewParams,
	key uuid.UUID,
) (model.CreateTripResult, error) {
	requestHash := hashParams(params)

	existing, found, err := u.lookup(ctx, key, requestHash)
	if err != nil {
		return model.CreateTripResult{}, err
	}

	if found {
		return model.CreateTripResult{Trip: existing}, nil
	}

	err = u.txManager.Do(ctx, func(ctx context.Context) error {
		claimed, err := u.repository.ClaimIdempotencyKey(ctx, key, requestHash, trip.ID, idempotencyKeyTTL)
		if err != nil {
			return err
		}

		if !claimed {
			return errIdempotencyKeyLost
		}

		return u.insert(ctx, trip)
	})

	if errors.Is(err, errIdempotencyKeyLost) {
		return u.replayAfterLostClaim(ctx, key, requestHash)
	}

	if err != nil {
		return model.CreateTripResult{}, err
	}

	u.created(ctx, trip)

	return model.CreateTripResult{Trip: trip, Created: true}, nil
}

func (u *Usecase) replayAfterLostClaim(
	ctx context.Context,
	key uuid.UUID,
	requestHash string,
) (model.CreateTripResult, error) {
	existing, found, err := u.lookup(ctx, key, requestHash)
	if err != nil {
		return model.CreateTripResult{}, err
	}

	if !found {
		return model.CreateTripResult{}, fmt.Errorf("idempotency key %s disappeared after conflict", key)
	}

	return model.CreateTripResult{Trip: existing}, nil
}

func (u *Usecase) lookup(
	ctx context.Context,
	key uuid.UUID,
	requestHash string,
) (tripdomain.Trip, bool, error) {
	record, found, err := u.repository.GetIdempotencyKey(ctx, key, idempotencyKeyTTL)
	if err != nil {
		return tripdomain.Trip{}, false, err
	}

	if !found {
		return tripdomain.Trip{}, false, nil
	}

	if record.RequestHash != requestHash {
		return tripdomain.Trip{}, false, tripdomain.ErrIdempotencyConflict
	}

	trip, err := u.repository.GetByID(ctx, record.TripID)
	if err != nil {
		return tripdomain.Trip{}, false, err
	}

	return trip, true, nil
}

func (u *Usecase) insert(ctx context.Context, trip tripdomain.Trip) error {
	if err := u.repository.Save(ctx, trip); err != nil {
		return err
	}

	return u.repository.AddCreatedHistory(ctx, trip.ID)
}

func (u *Usecase) created(ctx context.Context, trip tripdomain.Trip) {
	u.metrics.TripCreated(ctx)
	u.logger.InfoContext(ctx, "trip created", slog.String("trip_id", trip.ID.String()))
}
