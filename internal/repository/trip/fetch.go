package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
)

func (r *Repository) GetByID(ctx context.Context, tripID uuid.UUID) (tripdomain.Trip, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := squirrel.
		Select(tripColumns...).
		From("trips").
		Where(squirrel.Eq{"id": tripID}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return tripdomain.Trip{}, fmt.Errorf("build get trip query: %w", err)
	}

	trip, err := scanTrip(r.executor(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return tripdomain.Trip{}, tripdomain.ErrTripNotFound
	}

	if err != nil {
		return tripdomain.Trip{}, fmt.Errorf("get trip: %w", err)
	}

	return trip, nil
}

func (r *Repository) CountActive(ctx context.Context) (int64, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := squirrel.
		Select("count(*)").
		From("trips").
		Where(squirrel.Eq{columnStatus: string(tripdomain.StatusActive)}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build count active trips query: %w", err)
	}

	var count int64

	if err := r.executor(ctx).QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}

	return count, nil
}
