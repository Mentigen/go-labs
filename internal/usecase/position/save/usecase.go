package save

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/domain/trip"
)

type Usecase struct {
	repository   repository
	transactions transactions
	pending      pendingRequests
	metrics      metrics
	now          func() time.Time
}

func New(
	repository repository,
	transactions transactions,
	pending pendingRequests,
	metrics metrics,
	now func() time.Time,
) *Usecase {
	return &Usecase{
		repository:   repository,
		transactions: transactions,
		pending:      pending,
		metrics:      metrics,
		now:          now,
	}
}

func (u *Usecase) Execute(ctx context.Context, id uuid.UUID, input trip.PositionInput) (trip.Position, error) {
	position, err := trip.NewPosition(id, input, u.now())
	if err != nil {
		return trip.Position{}, err
	}
	err = u.transactions.Do(ctx, func(ctx context.Context) error {
		saved, err := u.repository.SavePosition(ctx, position)
		if err != nil {
			return err
		}
		position = saved
		return nil
	})
	if err != nil {
		return trip.Position{}, err
	}
	u.pending.Cancel(id)
	u.metrics.PositionSaved(ctx)
	return position, nil
}
