package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
)

func (r *Repository) Save(ctx context.Context, trip tripdomain.Trip) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := squirrel.
		Insert("trips").
		Columns(tripColumns...).
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
		return fmt.Errorf("build save trip query: %w", err)
	}

	_, err = r.executor(ctx).Exec(ctx, query, args...)
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == oneActivePerDriverConstraint {
		return tripdomain.ErrDriverBusy
	}

	return fmt.Errorf("save trip: %w", err)
}

func (r *Repository) Finish(ctx context.Context, trip tripdomain.Trip) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := squirrel.
		Update("trips").
		Set(columnStatus, string(trip.Status)).
		Set("finished_at", trip.FinishedAt).
		Set("updated_at", trip.UpdatedAt).
		Where(squirrel.Eq{
			"id":         trip.ID,
			columnStatus: string(tripdomain.StatusActive),
		}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build finish trip query: %w", err)
	}

	result, err := r.executor(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("finish trip: %w", err)
	}

	if result.RowsAffected() == 0 {
		return tripdomain.ErrTripCompleted
	}

	return nil
}
