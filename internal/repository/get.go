package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Mentigen/go-labs/internal/model"
	"github.com/Mentigen/go-labs/internal/txmanager"
)

func (r *TripRepository) GetByID(ctx context.Context, tripID uuid.UUID) (model.Trip, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := squirrel.
		Select(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude",
			"end_latitude", "end_longitude",
			"price", "status",
			"started_at", "finished_at",
			"created_at", "updated_at",
		).
		From("trips").
		Where(squirrel.Eq{"id": tripID}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return model.Trip{}, fmt.Errorf("build get trip query: %w", err)
	}

	var trip model.Trip

	executor := txmanager.ExecutorFromContext(ctx, r.pool)
	row := executor.QueryRow(ctx, query, args...)

	err = row.Scan(
		&trip.ID, &trip.UserID, &trip.DriverID,
		&trip.StartPoint.Latitude, &trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude, &trip.EndPoint.Longitude,
		&trip.Price, &trip.Status,
		&trip.StartedAt, &trip.FinishedAt,
		&trip.CreatedAt, &trip.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return model.Trip{}, model.ErrTripNotFound
	}

	if err != nil {
		return model.Trip{}, fmt.Errorf("get trip: %w", err)
	}

	return trip, nil
}
