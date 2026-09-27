package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"

	api "github.com/Mentigen/go-labs/api"
	"github.com/Mentigen/go-labs/internal/model"
	"github.com/Mentigen/go-labs/internal/service"
)

var requiredCreateTripFields = []string{"user_id", "driver_id", "start_point", "end_point", "price"}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeInvalidRequest(w, r, "failed to read request body")
		return
	}

	var presence map[string]json.RawMessage
	if err := json.Unmarshal(raw, &presence); err != nil {
		writeInvalidRequest(w, r, "invalid JSON body: "+err.Error())
		return
	}

	for _, field := range requiredCreateTripFields {
		if _, ok := presence[field]; !ok {
			writeInvalidRequest(w, r, fmt.Sprintf("%s is required", field))
			return
		}
	}

	var body api.CreateTripJSONRequestBody

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&body); err != nil {
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

	if !coordinatesValid(body.StartPoint) {
		writeInvalidRequest(w, r, "start_point contains invalid coordinates")
		return
	}

	if !coordinatesValid(body.EndPoint) {
		writeInvalidRequest(w, r, "end_point contains invalid coordinates")
		return
	}

	if body.Price < 0 {
		writeInvalidRequest(w, r, "price must not be negative")
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

func coordinatesValid(c api.Coordinates) bool {
	return c.Latitude >= -90 && c.Latitude <= 90 &&
		c.Longitude >= -180 && c.Longitude <= 180
}
