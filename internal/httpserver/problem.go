package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/routers"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	api "github.com/Mentigen/go-labs/api"
)

func writeProblem(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	problemType string,
	title string,
	detail string,
	code string,
) {
	instance := r.URL.Path

	problem := api.Problem{
		Type:     problemType,
		Title:    title,
		Status:   int32(status),
		Detail:   &detail,
		Instance: &instance,
		Code:     code,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(problem)
}

func writeInvalidRequest(w http.ResponseWriter, r *http.Request, detail string) {
	writeProblem(
		w, r,
		http.StatusBadRequest,
		"https://tripgo.example/problems/invalid-request",
		"Invalid request",
		detail,
		"invalid_request",
	)
}

func writeValidationError(
	_ context.Context,
	err error,
	w http.ResponseWriter,
	r *http.Request,
	opts nethttpmiddleware.ErrorHandlerOpts,
) {
	switch {
	case errors.Is(err, routers.ErrMethodNotAllowed):
		writeProblem(
			w, r,
			http.StatusMethodNotAllowed,
			"https://tripgo.example/problems/method-not-allowed",
			"Method not allowed",
			"Method is not allowed for this path",
			"method_not_allowed",
		)
	case opts.StatusCode == http.StatusNotFound:
		writeProblem(
			w, r,
			http.StatusNotFound,
			"https://tripgo.example/problems/not-found",
			"Not found",
			"Path was not found",
			"not_found",
		)
	case opts.StatusCode == http.StatusBadRequest:
		writeInvalidRequest(w, r, firstLine(err.Error()))
	default:
		writeProblem(
			w, r,
			http.StatusInternalServerError,
			"https://tripgo.example/problems/internal-error",
			"Internal error",
			"Internal server error",
			"internal_error",
		)
	}
}

func firstLine(message string) string {
	line, _, _ := strings.Cut(message, "\n")
	return line
}
