package httpserver

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	api "github.com/Mentigen/go-labs/api"
)

func NewRouter(handler api.ServerInterface) (*chi.Mux, error) {
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}

	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		DoNotValidateServers: true,
		ErrorHandlerWithOpts: writeValidationError,
	})

	router := chi.NewRouter()
	router.Use(validator)

	api.HandlerWithOptions(handler, api.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeInvalidRequest(w, r, err.Error())
		},
	})

	return router, nil
}
