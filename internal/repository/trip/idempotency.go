package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Mentigen/go-labs/internal/model"
)

func (r *Repository) GetIdempotencyKey(
	ctx context.Context,
	key uuid.UUID,
	ttl time.Duration,
) (model.IdempotencyRecord, bool, error) {
	ctx, cancel := r.withTimeout(ctx)
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
		return model.IdempotencyRecord{}, false, fmt.Errorf("build get idempotency key query: %w", err)
	}

	var record model.IdempotencyRecord

	err = r.executor(ctx).QueryRow(ctx, query, args...).Scan(&record.RequestHash, &record.TripID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.IdempotencyRecord{}, false, nil
	}

	if err != nil {
		return model.IdempotencyRecord{}, false, fmt.Errorf("get idempotency key: %w", err)
	}

	return record, true, nil
}

func (r *Repository) ClaimIdempotencyKey(
	ctx context.Context,
	key uuid.UUID,
	requestHash string,
	tripID uuid.UUID,
	ttl time.Duration,
) (bool, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	cutoff := time.Now().Add(-ttl)

	query, args, err := squirrel.
		Insert("idempotency_keys").
		Columns("key", "request_hash", "trip_id").
		Values(key, requestHash, tripID).
		Suffix(
			"ON CONFLICT (key) DO UPDATE SET request_hash = EXCLUDED.request_hash, "+
				"trip_id = EXCLUDED.trip_id, created_at = now() "+
				"WHERE idempotency_keys.created_at < ?",
			cutoff,
		).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build claim idempotency key query: %w", err)
	}

	tag, err := r.executor(ctx).Exec(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("claim idempotency key: %w", err)
	}

	return tag.RowsAffected() == 1, nil
}
