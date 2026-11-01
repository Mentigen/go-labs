package create

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
)

type hashPayload struct {
	UserID     uuid.UUID `json:"user_id"`
	DriverID   uuid.UUID `json:"driver_id"`
	StartPoint point     `json:"start_point"`
	EndPoint   point     `json:"end_point"`
	Price      int64     `json:"price"`
}

type point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func hashParams(params tripdomain.NewParams) string {
	raw, _ := json.Marshal(hashPayload{
		UserID:     params.UserID,
		DriverID:   params.DriverID,
		StartPoint: point(params.StartPoint),
		EndPoint:   point(params.EndPoint),
		Price:      params.Price,
	})

	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:])
}
