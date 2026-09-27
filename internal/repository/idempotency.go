package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Mentigen/go-labs/internal/txmanager"
)

type IdempotencyRecord struct {
	RequestHash string
	TripID      uuid.UUID
}

func (r *TripRepository) GetIdempotencyKey(
	ctx context.Context,
	key uuid.UUID,
	ttl time.Duration,
) (IdempotencyRecord, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	cutoff := time.Now().Add(-ttl)

	query, args, err := squirrel.
		Select("request_hash", "trip_id").
		From("idempotency_keys").
		Where(squirrel.Eq{"key": key}).
		Where(squirrel.GtOrEq{"created_at": cutoff}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("build get idempotency key query: %w", err)
	}

	executor := txmanager.ExecutorFromContext(ctx, r.pool)

	var record IdempotencyRecord

	err = executor.QueryRow(ctx, query, args...).Scan(&record.RequestHash, &record.TripID)

	if errors.Is(err, pgx.ErrNoRows) {
		return IdempotencyRecord{}, false, nil
	}

	if err != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("get idempotency key: %w", err)
	}

	return record, true, nil
}

func (r *TripRepository) ClaimIdempotencyKey(
	ctx context.Context,
	key uuid.UUID,
	requestHash string,
	tripID uuid.UUID,
) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := squirrel.
		Insert("idempotency_keys").
		Columns("key", "request_hash", "trip_id").
		Values(key, requestHash, tripID).
		Suffix("ON CONFLICT (key) DO NOTHING").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build claim idempotency key query: %w", err)
	}

	executor := txmanager.ExecutorFromContext(ctx, r.pool)

	tag, err := executor.Exec(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("claim idempotency key: %w", err)
	}

	return tag.RowsAffected() == 1, nil
}
