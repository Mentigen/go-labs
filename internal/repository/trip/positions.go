package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
)

func (r *Repository) SavePosition(ctx context.Context, position tripdomain.Position) (tripdomain.Position, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	executor := r.executor(ctx)
	var status tripdomain.Status
	err := executor.QueryRow(ctx, "SELECT status FROM trips WHERE id=$1 FOR UPDATE", position.TripID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return tripdomain.Position{}, tripdomain.ErrTripNotFound
	}
	if err != nil {
		return tripdomain.Position{}, fmt.Errorf("lock trip for position: %w", err)
	}
	if status != tripdomain.StatusActive {
		return tripdomain.Position{}, tripdomain.ErrTripCompleted
	}
	err = executor.QueryRow(ctx, `INSERT INTO trip_positions (trip_id, latitude, longitude, recorded_at)
        VALUES ($1,$2,$3,$4) RETURNING id, recorded_at`, position.TripID, position.Coordinates.Latitude,
		position.Coordinates.Longitude, position.RecordedAt).Scan(&position.ID, &position.RecordedAt)
	if err != nil {
		return tripdomain.Position{}, fmt.Errorf("insert position: %w", err)
	}
	_, err = executor.Exec(ctx, `UPDATE trips SET last_position_at=GREATEST(last_position_at,$2),
        updated_at=GREATEST(updated_at + interval '1 microsecond',clock_timestamp()) WHERE id=$1`, position.TripID, position.RecordedAt)
	if err != nil {
		return tripdomain.Position{}, fmt.Errorf("update last position: %w", err)
	}
	return position, nil
}

func (r *Repository) ListPositions(ctx context.Context, id uuid.UUID) ([]tripdomain.Position, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	if _, err := r.GetByID(ctx, id); err != nil {
		return nil, err
	}
	rows, err := r.executor(ctx).Query(ctx, `SELECT id,trip_id,latitude,longitude,recorded_at
        FROM trip_positions WHERE trip_id=$1 ORDER BY recorded_at,id`, id)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	defer rows.Close()
	positions := make([]tripdomain.Position, 0)
	for rows.Next() {
		var p tripdomain.Position
		if err := rows.Scan(&p.ID, &p.TripID, &p.Coordinates.Latitude, &p.Coordinates.Longitude, &p.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		positions = append(positions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read positions: %w", err)
	}
	return positions, nil
}
