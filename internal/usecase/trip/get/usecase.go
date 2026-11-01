package get

import (
	"context"

	"github.com/google/uuid"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
)

type Usecase struct {
	repository tripRepository
}

func New(repository tripRepository) *Usecase {
	return &Usecase{repository: repository}
}

func (u *Usecase) Execute(ctx context.Context, tripID uuid.UUID) (tripdomain.Trip, error) {
	return u.repository.GetByID(ctx, tripID)
}
