package create

import (
	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/generated"
	"github.com/Mentigen/go-labs/internal/model"
)

func toInput(body generated.CreateTripJSONRequestBody, key *uuid.UUID) model.CreateTripInput {
	return model.CreateTripInput{
		UserID:   body.UserId,
		DriverID: body.DriverId,
		StartPoint: trip.Coordinates{
			Latitude:  body.StartPoint.Latitude,
			Longitude: body.StartPoint.Longitude,
		},
		EndPoint: trip.Coordinates{
			Latitude:  body.EndPoint.Latitude,
			Longitude: body.EndPoint.Longitude,
		},
		Price:          body.Price,
		IdempotencyKey: key,
	}
}
