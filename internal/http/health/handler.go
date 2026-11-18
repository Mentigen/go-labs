package health

import (
	"context"
	"net/http"
	"time"

	"github.com/Mentigen/go-labs/internal/generated"
	"github.com/Mentigen/go-labs/internal/http/response"
)

type Handler struct {
	database pinger
	timeout  time.Duration
}

func New(database pinger, timeout time.Duration) *Handler {
	return &Handler{database: database, timeout: timeout}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, generated.HealthResponse{Status: generated.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	if err := h.database.Ping(ctx); err != nil {
		response.JSON(w, http.StatusServiceUnavailable, generated.HealthResponse{Status: generated.Unavailable})
		return
	}

	response.JSON(w, http.StatusOK, generated.HealthResponse{Status: generated.Ok})
}
