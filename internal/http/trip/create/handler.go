package create

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/Mentigen/go-labs/internal/generated"
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

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, params generated.CreateTripParams) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		domainerror.Write(w, r, h.logger, domainerror.NewRequestError("failed to read request body"))
		return
	}

	var body generated.CreateTripJSONRequestBody

	if err := json.Unmarshal(raw, &body); err != nil {
		domainerror.Write(w, r, h.logger, domainerror.NewRequestError("invalid JSON body: "+err.Error()))
		return
	}

	result, err := h.usecase.Execute(r.Context(), toInput(body, params.IdempotencyKey))
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	status := http.StatusOK

	if result.Created {
		status = http.StatusCreated
		w.Header().Set("Location", "/api/v1/trips/"+result.Trip.ID.String())
	}

	response.JSON(w, status, dto.FromTrip(result.Trip))
}
