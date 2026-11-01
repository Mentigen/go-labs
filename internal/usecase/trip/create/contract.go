package create

import (
	"context"
	"time"

	"github.com/google/uuid"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/model"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type (
	tripRepository interface {
		Save(ctx context.Context, trip tripdomain.Trip) error
		AddCreatedHistory(ctx context.Context, tripID uuid.UUID) error
		GetByID(ctx context.Context, tripID uuid.UUID) (tripdomain.Trip, error)
		GetIdempotencyKey(ctx context.Context, key uuid.UUID, ttl time.Duration) (model.IdempotencyRecord, bool, error)
		ClaimIdempotencyKey(
			ctx context.Context,
			key uuid.UUID,
			requestHash string,
			tripID uuid.UUID,
			ttl time.Duration,
		) (bool, error)
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	metrics interface {
		TripCreated(ctx context.Context)
	}
)
