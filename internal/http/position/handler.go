package position

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/generated"
	"github.com/Mentigen/go-labs/internal/http/domainerror"
	"github.com/Mentigen/go-labs/internal/http/response"
)

type Handler struct {
	save   saver
	list   lister
	logger *slog.Logger
}

func New(save saver, list lister, logger *slog.Logger) *Handler {
	return &Handler{save: save, list: list, logger: logger}
}

func (h *Handler) Save(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		domainerror.Write(w, r, h.logger, domainerror.NewRequestError("failed to read position body"))
		return
	}
	var body generated.CreateTripPositionJSONRequestBody
	if err := json.Unmarshal(raw, &body); err != nil {
		domainerror.Write(w, r, h.logger, domainerror.NewRequestError("invalid position body"))
		return
	}
	saved, err := h.save.Execute(r.Context(), id, trip.PositionInput{
		Coordinates: trip.Coordinates{Latitude: body.Coordinates.Latitude, Longitude: body.Coordinates.Longitude},
		RecordedAt:  body.RecordedAt,
	})
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}
	response.JSON(w, http.StatusCreated, toDTO(saved))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	positions, err := h.list.Execute(r.Context(), id)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}
	result := make([]generated.TripPosition, 0, len(positions))
	for _, p := range positions {
		result = append(result, toDTO(p))
	}
	response.JSON(w, http.StatusOK, generated.TripPositionsResponse{Positions: result})
}

func toDTO(p trip.Position) generated.TripPosition {
	return generated.TripPosition{
		Id:          p.ID,
		TripId:      p.TripID,
		RecordedAt:  p.RecordedAt,
		Coordinates: generated.Coordinates{Latitude: p.Coordinates.Latitude, Longitude: p.Coordinates.Longitude},
	}
}
