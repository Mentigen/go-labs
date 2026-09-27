package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Mentigen/go-labs/internal/model"
	"github.com/Mentigen/go-labs/internal/txmanager"
)

func (r *TripRepository) Create(ctx context.Context, trip model.Trip) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := squirrel.
		Insert("trips").
		Columns(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude",
			"end_latitude", "end_longitude",
			"price", "status",
			"started_at", "finished_at",
			"created_at", "updated_at",
		).
		Values(
			trip.ID, trip.UserID, trip.DriverID,
			trip.StartPoint.Latitude, trip.StartPoint.Longitude,
			trip.EndPoint.Latitude, trip.EndPoint.Longitude,
			trip.Price, string(trip.Status),
			trip.StartedAt, trip.FinishedAt,
			trip.CreatedAt, trip.UpdatedAt,
		).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create trip query: %w", err)
	}

	executor := txmanager.ExecutorFromContext(ctx, r.pool)

	_, err = executor.Exec(ctx, query, args...)
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "trips_one_active_per_driver_idx" {
		return model.ErrDriverBusy
	}

	return fmt.Errorf("create trip: %w", err)
}
