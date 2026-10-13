package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/model"
	"github.com/Mentigen/go-labs/internal/txmanager"
)

func (r *TripRepository) Finish(ctx context.Context, tripID uuid.UUID, finishedAt time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := squirrel.
		Update("trips").
		Set("status", string(model.TripStatusCompleted)).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(squirrel.Eq{
			"id":     tripID,
			"status": string(model.TripStatusActive),
		}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build finish trip query: %w", err)
	}

	executor := txmanager.ExecutorFromContext(ctx, r.pool)

	result, err := executor.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("finish trip: %w", err)
	}

	if result.RowsAffected() == 1 {
		return nil
	}

	_, err = r.GetByID(ctx, tripID)
	if err != nil {
		return err
	}

	return model.ErrTripCompleted
}
