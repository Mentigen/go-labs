package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/domain/trip"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type repository interface {
	ListPositions(context.Context, uuid.UUID) ([]trip.Position, error)
}
