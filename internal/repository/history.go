package repository

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/txmanager"
)

func (r *TripRepository) AddCreatedHistory(ctx context.Context, tripID uuid.UUID) error {
	return r.addHistory(ctx, tripID, nil, "active", "trip created")
}

func (r *TripRepository) AddCompletedHistory(ctx context.Context, tripID uuid.UUID) error {
	fromStatus := "active"
	return r.addHistory(ctx, tripID, &fromStatus, "completed", "trip finished")
}

func (r *TripRepository) addHistory(
	ctx context.Context,
	tripID uuid.UUID,
	fromStatus *string,
	toStatus string,
	reason string,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := squirrel.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, fromStatus, toStatus, reason).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build add history query: %w", err)
	}

	executor := txmanager.ExecutorFromContext(ctx, r.pool)

	_, err = executor.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("add history: %w", err)
	}

	return nil
}
