package trip

import (
	"github.com/jackc/pgx/v5"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
)

const columnStatus = "status"

var tripColumns = []string{
	"id", "user_id", "driver_id",
	"start_latitude", "start_longitude",
	"end_latitude", "end_longitude",
	"price", columnStatus,
	"started_at", "finished_at",
	"created_at", "updated_at",
}

func scanTrip(row pgx.Row) (tripdomain.Trip, error) {
	var trip tripdomain.Trip

	err := row.Scan(
		&trip.ID, &trip.UserID, &trip.DriverID,
		&trip.StartPoint.Latitude, &trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude, &trip.EndPoint.Longitude,
		&trip.Price, &trip.Status,
		&trip.StartedAt, &trip.FinishedAt,
		&trip.CreatedAt, &trip.UpdatedAt,
	)

	return trip, err
}
