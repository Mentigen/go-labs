package dto

import (
	"github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/generated"
)

func FromTrip(source trip.Trip) generated.Trip {
	return generated.Trip{
		Id:       source.ID,
		UserId:   source.UserID,
		DriverId: source.DriverID,

		StartPoint: generated.Coordinates{
			Latitude:  source.StartPoint.Latitude,
			Longitude: source.StartPoint.Longitude,
		},
		EndPoint: generated.Coordinates{
			Latitude:  source.EndPoint.Latitude,
			Longitude: source.EndPoint.Longitude,
		},

		Price:  source.Price,
		Status: generated.TripStatus(source.Status),

		StartedAt:  source.StartedAt,
		FinishedAt: source.FinishedAt,
	}
}
