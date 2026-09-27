package httpserver

import (
	"encoding/json"
	"net/http"

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
