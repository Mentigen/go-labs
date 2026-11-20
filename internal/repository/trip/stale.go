package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/Mentigen/go-labs/internal/model"
)

func (r *Repository) StaleTrips(ctx context.Context, staleAfter time.Duration, cutoff time.Time, cursor *model.PositionCursor, limit int) (model.StalePage, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	var cutoffArg any
	if !cutoff.IsZero() {
		cutoffArg = cutoff
	}
	const boundary = "COALESCE($1::timestamptz, statement_timestamp() - make_interval(secs => $3))"
	query := `SELECT id, driver_id, COALESCE(last_position_at, started_at), updated_at, ` + boundary + `
		FROM trips
		WHERE status = 'active' AND COALESCE(last_position_at, started_at) < ` + boundary
	args := []any{cutoffArg, limit, staleAfter.Seconds()}
	if cursor != nil {
		query += " AND (COALESCE(last_position_at,started_at),id) > ($4,$5)"
		args = append(args, cursor.PositionAt, cursor.ID)
	}
	query += " ORDER BY COALESCE(last_position_at,started_at),id LIMIT $2"
	rows, err := r.executor(ctx).Query(ctx, query, args...)
	if err != nil {
		return model.StalePage{}, fmt.Errorf("select stale trips: %w", err)
	}
	defer rows.Close()
	page := model.StalePage{Cutoff: cutoff}
	for rows.Next() {
		var trip model.StaleTrip
		if err := rows.Scan(&trip.ID, &trip.DriverID, &trip.PositionAt, &trip.UpdatedAt, &page.Cutoff); err != nil {
			return model.StalePage{}, fmt.Errorf("scan stale trip: %w", err)
		}
		page.Trips = append(page.Trips, trip)
	}
	if err := rows.Err(); err != nil {
		return model.StalePage{}, fmt.Errorf("read stale trips: %w", err)
	}
	return page, nil
}

func (r *Repository) PositionRequestNeeded(ctx context.Context, trip model.StaleTrip) (bool, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	var needed bool
	err := r.executor(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM trips
        WHERE id=$1 AND status='active' AND updated_at=$2
        AND COALESCE(last_position_at,started_at)=$3)`, trip.ID, trip.UpdatedAt, trip.PositionAt).Scan(&needed)
	if err != nil {
		return false, fmt.Errorf("check position request: %w", err)
	}
	return needed, nil
}
