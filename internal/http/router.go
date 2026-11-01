package httpserver

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mentigen/go-labs/internal/generated"
	"github.com/Mentigen/go-labs/internal/http/domainerror"
	"github.com/Mentigen/go-labs/internal/http/health"
	"github.com/Mentigen/go-labs/internal/http/middleware"
	"github.com/Mentigen/go-labs/internal/http/trip/create"
	"github.com/Mentigen/go-labs/internal/http/trip/finish"
	"github.com/Mentigen/go-labs/internal/http/trip/get"
)

type Handlers struct {
	Create *create.Handler
	Get    *get.Handler
	Finish *finish.Handler
	Health *health.Handler
}

type Options struct {
	Handlers       Handlers
	Logger         *slog.Logger
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Propagator     propagation.TextMapPropagator
}

type api struct {
	handlers Handlers
}

func (a api) CreateTrip(w http.ResponseWriter, r *http.Request, params generated.CreateTripParams) {
	a.handlers.Create.Handle(w, r, params)
}

func (a api) GetTrip(w http.ResponseWriter, r *http.Request, tripID generated.TripId) {
	a.handlers.Get.Handle(w, r, tripID)
}

func (a api) FinishTrip(w http.ResponseWriter, r *http.Request, tripID generated.TripId) {
	a.handlers.Finish.Handle(w, r, tripID)
}

func (a api) Health(w http.ResponseWriter, r *http.Request) {
	a.handlers.Health.Health(w, r)
}

func (a api) Ready(w http.ResponseWriter, r *http.Request) {
	a.handlers.Health.Ready(w, r)
}

func NewRouter(options Options) (http.Handler, error) {
	spec, err := generated.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}

	router := chi.NewRouter()

	resolve := func(r *http.Request) string {
		return router.Find(chi.NewRouteContext(), r.Method, r.URL.Path)
	}

	metrics, err := middleware.Metrics(options.MeterProvider, resolve)
	if err != nil {
		return nil, fmt.Errorf("create metrics middleware: %w", err)
	}

	router.Use(
		middleware.Recover(options.Logger),
		middleware.Tracing(options.TracerProvider, options.Propagator, resolve),
		middleware.Logging(options.Logger, resolve),
		metrics,
		middleware.OpenAPIValidator(spec, options.Logger),
	)

	generated.HandlerWithOptions(api{handlers: options.Handlers}, generated.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			domainerror.Write(w, r, options.Logger, domainerror.NewRequestError(err.Error()))
		},
	})

	return router, nil
}
