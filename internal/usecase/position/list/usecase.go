package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/domain/trip"
)

type Usecase struct {
	repository repository
}

func New(repository repository) *Usecase {
	return &Usecase{repository: repository}
}

func (u *Usecase) Execute(ctx context.Context, id uuid.UUID) ([]trip.Position, error) {
	return u.repository.ListPositions(ctx, id)
}
