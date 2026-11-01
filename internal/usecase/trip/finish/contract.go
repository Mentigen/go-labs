package finish

import (
	"context"

	"github.com/google/uuid"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type (
	tripRepository interface {
		GetByID(ctx context.Context, tripID uuid.UUID) (tripdomain.Trip, error)
		Finish(ctx context.Context, trip tripdomain.Trip) error
		AddCompletedHistory(ctx context.Context, tripID uuid.UUID) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	metrics interface {
		TripCompleted(ctx context.Context)
	}
)
