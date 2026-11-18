package trip

import (
	"context"
	"time"

	"github.com/Mentigen/go-labs/internal/infra/postgres/transaction"
)

const (
	uniqueViolationCode          = "23505"
	oneActivePerDriverConstraint = "trips_one_active_per_driver_idx"
)

type Repository struct {
	db           database
	queryTimeout time.Duration
}

func New(db database, queryTimeout time.Duration) *Repository {
	return &Repository{
		db:           db,
		queryTimeout: queryTimeout,
	}
}

func (r *Repository) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.queryTimeout)
}

func (r *Repository) executor(ctx context.Context) transaction.Executor {
	return transaction.ExecutorFromContext(ctx, r.db)
}
