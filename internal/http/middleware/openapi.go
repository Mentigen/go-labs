package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/Mentigen/go-labs/internal/http/domainerror"
)

func OpenAPIValidator(spec *openapi3.T, logger *slog.Logger) func(http.Handler) http.Handler {
	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		DoNotValidateServers: true,
		ErrorHandlerWithOpts: func(
			_ context.Context,
			err error,
			w http.ResponseWriter,
			r *http.Request,
			opts nethttpmiddleware.ErrorHandlerOpts,
		) {
			domainerror.Write(w, r, logger, validationFailure(err, opts))
		},
	})
}

func validationFailure(err error, opts nethttpmiddleware.ErrorHandlerOpts) error {
	switch {
	case errors.Is(err, routers.ErrMethodNotAllowed):
		return domainerror.ErrMethodNotAllowed
	case opts.StatusCode == http.StatusNotFound:
		return domainerror.ErrRouteNotFound
	case opts.StatusCode == http.StatusBadRequest:
		line, _, _ := strings.Cut(err.Error(), "\n")
		return domainerror.NewRequestError(line)
	default:
		return err
	}
}
