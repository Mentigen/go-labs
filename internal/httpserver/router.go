package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	api "github.com/Mentigen/go-labs/api"
)

func NewRouter(handler api.ServerInterface) *chi.Mux {
	router := chi.NewRouter()

	api.HandlerWithOptions(handler, api.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeInvalidRequest(w, r, err.Error())
		},
	})

	return router
}
