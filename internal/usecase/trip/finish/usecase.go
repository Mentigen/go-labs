package finish

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
)

type Usecase struct {
	repository tripRepository
	txManager  txManager
	metrics    metrics
	logger     *slog.Logger
	now        func() time.Time
}

func New(
	repository tripRepository,
	txManager txManager,
	metrics metrics,
	logger *slog.Logger,
	now func() time.Time,
) *Usecase {
	return &Usecase{
		repository: repository,
		txManager:  txManager,
		metrics:    metrics,
		logger:     logger,
		now:        now,
	}
}

func (u *Usecase) Execute(ctx context.Context, tripID uuid.UUID) (tripdomain.Trip, error) {
	var finished tripdomain.Trip

	err := u.txManager.Do(ctx, func(ctx context.Context) error {
		trip, err := u.repository.GetByID(ctx, tripID)
		if err != nil {
			return err
		}

		if err := trip.Finish(u.now().UTC()); err != nil {
			return err
		}

		if err := u.repository.Finish(ctx, trip); err != nil {
			return err
		}

		if err := u.repository.AddCompletedHistory(ctx, tripID); err != nil {
			return err
		}

		finished = trip

		return nil
	})
	if err != nil {
		return tripdomain.Trip{}, err
	}

	u.metrics.TripCompleted(ctx)
	u.logger.InfoContext(ctx, "trip completed", slog.String("trip_id", tripID.String()))

	return finished, nil
}
