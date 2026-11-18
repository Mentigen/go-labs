package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/domain/trip"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type usecase interface {
	Execute(ctx context.Context, tripID uuid.UUID) (trip.Trip, error)
}
