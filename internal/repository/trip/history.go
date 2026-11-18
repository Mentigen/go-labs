package trip

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
)

func (r *Repository) AddCreatedHistory(ctx context.Context, tripID uuid.UUID) error {
	return r.addHistory(ctx, tripID, nil, tripdomain.StatusActive, "trip created")
}

func (r *Repository) AddCompletedHistory(ctx context.Context, tripID uuid.UUID) error {
	from := tripdomain.StatusActive

	return r.addHistory(ctx, tripID, &from, tripdomain.StatusCompleted, "trip finished")
}

func (r *Repository) addHistory(
	ctx context.Context,
	tripID uuid.UUID,
	from *tripdomain.Status,
	to tripdomain.Status,
	reason string,
) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	var fromStatus *string

	if from != nil {
		value := string(*from)
		fromStatus = &value
	}

	query, args, err := squirrel.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, fromStatus, string(to), reason).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build add history query: %w", err)
	}

	if _, err := r.executor(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("add history: %w", err)
	}

	return nil
}
