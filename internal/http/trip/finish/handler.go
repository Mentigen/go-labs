package finish

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/http/domainerror"
	"github.com/Mentigen/go-labs/internal/http/response"
	"github.com/Mentigen/go-labs/internal/http/trip/dto"
)

type Handler struct {
	usecase usecase
	logger  *slog.Logger
}

func New(usecase usecase, logger *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: logger}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, tripID uuid.UUID) {
	found, err := h.usecase.Execute(r.Context(), tripID)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	response.JSON(w, http.StatusOK, dto.FromTrip(found))
}
