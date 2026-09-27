package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	api "github.com/Mentigen/go-labs/api"
	"github.com/Mentigen/go-labs/internal/service"
)

type Handler struct {
	api.Unimplemented

	pingDatabase func(context.Context) error
	queryTimeout time.Duration
	tripService  *service.TripService
}

func NewHandler(
	pingDatabase func(context.Context) error,
	queryTimeout time.Duration,
	tripService *service.TripService,
) *Handler {
	return &Handler{
		pingDatabase: pingDatabase,
		queryTimeout: queryTimeout,
		tripService:  tripService,
	}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	if err := h.pingDatabase(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}

	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
