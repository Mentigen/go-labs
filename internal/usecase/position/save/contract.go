package save

import (
	"context"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/domain/trip"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type repository interface {
	SavePosition(context.Context, trip.Position) (trip.Position, error)
}
type transactions interface {
	Do(context.Context, func(context.Context) error) error
}
type (
	pendingRequests interface{ Cancel(uuid.UUID) }
	metrics         interface{ PositionSaved(context.Context) }
)
