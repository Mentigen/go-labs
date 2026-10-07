package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"

	api "github.com/Mentigen/go-labs/api"
	"github.com/Mentigen/go-labs/internal/model"
	"github.com/Mentigen/go-labs/internal/service"
)

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeInvalidRequest(w, r, "failed to read request body")
		return
	}

	var body api.CreateTripJSONRequestBody

	if err := json.Unmarshal(raw, &body); err != nil {
		writeInvalidRequest(w, r, "invalid JSON body: "+err.Error())
		return
	}

	if body.UserId == uuid.Nil {
		writeInvalidRequest(w, r, "user_id must not be empty")
		return
	}

	if body.DriverId == uuid.Nil {
		writeInvalidRequest(w, r, "driver_id must not be empty")
		return
	}

	input := service.CreateTripInput{
		UserID:   body.UserId,
		DriverID: body.DriverId,
		StartPoint: model.Coordinates{
			Latitude:  body.StartPoint.Latitude,
			Longitude: body.StartPoint.Longitude,
		},
		EndPoint: model.Coordinates{
			Latitude:  body.EndPoint.Latitude,
			Longitude: body.EndPoint.Longitude,
		},
		Price: body.Price,
	}

	var (
		trip    model.Trip
		created bool
	)

	if params.IdempotencyKey != nil {
		trip, created, err = h.tripService.CreateIdempotent(r.Context(), input, *params.IdempotencyKey)
	} else {
		created = true
		trip, err = h.tripService.Create(r.Context(), input)
	}

	if errors.Is(err, model.ErrDriverBusy) {
		writeProblem(
			w, r,
			http.StatusConflict,
			"https://tripgo.example/problems/driver-busy",
			"Driver busy",
			"Driver already has an active trip",
			"driver_busy",
		)
		return
	}

	if errors.Is(err, model.ErrIdempotencyConflict) {
		writeProblem(
			w, r,
			http.StatusConflict,
			"https://tripgo.example/problems/idempotency-conflict",
			"Idempotency conflict",
			"Idempotency-Key was already used with a different request body",
			"idempotency_conflict",
		)
		return
	}

	if err != nil {
		log.Printf("create trip: %v", err)
		writeProblem(
			w, r,
			http.StatusInternalServerError,
			"https://tripgo.example/problems/internal-error",
			"Internal error",
			"Internal server error",
			"internal_error",
		)
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
		w.Header().Set("Location", "/api/v1/trips/"+trip.ID.String())
	}

	writeJSON(w, status, tripToAPI(trip))
}
