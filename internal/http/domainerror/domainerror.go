package domainerror

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/generated"
)

const (
	problemBase = "https://tripgo.example/problems/"

	codeInvalidRequest       = "invalid_request"
	codeNotFound             = "not_found"
	codeMethodNotAllowed     = "method_not_allowed"
	codeTripNotFound         = "trip_not_found"
	codeTripCompleted        = "trip_completed"
	codeDriverBusy           = "driver_busy"
	codeIdempotencyConflict  = "idempotency_conflict"
	codeInternalError        = "internal_error"
	internalErrorDetail      = "Internal server error"
	titleInvalidRequest      = "Invalid request"
	titleNotFound            = "Not found"
	titleMethodNotAllowed    = "Method not allowed"
	titleTripNotFound        = "Trip not found"
	titleTripCompleted       = "Trip completed"
	titleDriverBusy          = "Driver busy"
	titleIdempotencyConflict = "Idempotency conflict"
	titleInternalError       = "Internal error"
)

var (
	ErrRouteNotFound    = errors.New("route not found")
	ErrMethodNotAllowed = errors.New("method not allowed")
)

type RequestError struct {
	Detail string
}

func NewRequestError(detail string) *RequestError {
	return &RequestError{Detail: detail}
}

func (e *RequestError) Error() string {
	return e.Detail
}

func Problem(err error) (int, generated.Problem) {
	var (
		validationError *trip.ValidationError
		requestError    *RequestError
	)

	switch {
	case errors.As(err, &validationError):
		return problem(http.StatusBadRequest, codeInvalidRequest, titleInvalidRequest, validationError.Error())
	case errors.As(err, &requestError):
		return problem(http.StatusBadRequest, codeInvalidRequest, titleInvalidRequest, requestError.Detail)
	}
	for _, mapping := range mappings {
		if errors.Is(err, mapping.err) {
			return problem(mapping.status, mapping.code, mapping.title, mapping.detail)
		}
	}
	return problem(http.StatusInternalServerError, codeInternalError, titleInternalError, internalErrorDetail)
}

type errorMapping struct {
	err                 error
	status              int
	code, title, detail string
}

var mappings = []errorMapping{
	{ErrRouteNotFound, http.StatusNotFound, codeNotFound, titleNotFound, "Path was not found"},
	{ErrMethodNotAllowed, http.StatusMethodNotAllowed, codeMethodNotAllowed, titleMethodNotAllowed, "Method is not allowed for this path"},
	{trip.ErrTripNotFound, http.StatusNotFound, codeTripNotFound, titleTripNotFound, "Trip with this id does not exist"},
	{trip.ErrTripCompleted, http.StatusConflict, codeTripCompleted, titleTripCompleted, "Trip is already completed"},
	{trip.ErrDriverBusy, http.StatusConflict, codeDriverBusy, titleDriverBusy, "Driver already has an active trip"},
	{trip.ErrIdempotencyConflict, http.StatusConflict, codeIdempotencyConflict, titleIdempotencyConflict, "Idempotency-Key was already used with a different request body"},
}

func Write(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	status, body := Problem(err)

	if status >= http.StatusInternalServerError {
		logger.ErrorContext(r.Context(), "request failed", slog.String("error", err.Error()))
	}

	WriteProblem(w, r, status, body)
}

func WriteProblem(w http.ResponseWriter, r *http.Request, status int, body generated.Problem) {
	instance := r.URL.Path
	body.Instance = &instance

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func problem(status int, code, title, detail string) (int, generated.Problem) {
	return status, generated.Problem{
		Type:   problemBase + slugOf(code),
		Title:  title,
		Status: int32(status), //nolint:gosec // HTTP status codes always fit into int32
		Detail: &detail,
		Code:   code,
	}
}

func slugOf(code string) string {
	return strings.ReplaceAll(code, "_", "-")
}
