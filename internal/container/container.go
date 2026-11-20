package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Mentigen/go-labs/internal/config"
	"github.com/Mentigen/go-labs/internal/gateway/push"
	httpserver "github.com/Mentigen/go-labs/internal/http"
	"github.com/Mentigen/go-labs/internal/http/health"
	"github.com/Mentigen/go-labs/internal/http/position"
	createhandler "github.com/Mentigen/go-labs/internal/http/trip/create"
	finishhandler "github.com/Mentigen/go-labs/internal/http/trip/finish"
	gethandler "github.com/Mentigen/go-labs/internal/http/trip/get"
	"github.com/Mentigen/go-labs/internal/infra/postgres"
	"github.com/Mentigen/go-labs/internal/infra/postgres/transaction"
	"github.com/Mentigen/go-labs/internal/infra/telemetry"
	"github.com/Mentigen/go-labs/internal/infra/triphash"
	triprepository "github.com/Mentigen/go-labs/internal/repository/trip"
	createusecase "github.com/Mentigen/go-labs/internal/usecase/trip/create"
	finishusecase "github.com/Mentigen/go-labs/internal/usecase/trip/finish"
	getusecase "github.com/Mentigen/go-labs/internal/usecase/trip/get"
	"github.com/Mentigen/go-labs/internal/worker/positions"
)

const transactionTracerName = "github.com/Mentigen/go-labs/internal/infra/postgres/transaction"

type Container struct {
	Logger *slog.Logger
	Server *http.Server

	pool      *pgxpool.Pool
	telemetry *telemetry.Telemetry
	positions *positions.Pool
	push      *push.Gateway
}

func New(ctx context.Context, cfg config.Config) (*Container, error) {
	logger := telemetry.NewLogger(os.Stdout, cfg.LogLevel, cfg.Telemetry.ServiceName)

	otel, err := telemetry.Setup(ctx, cfg.Telemetry, logger)
	if err != nil {
		return nil, fmt.Errorf("set up telemetry: %w", err)
	}

	pool, err := newPool(ctx, cfg.Database, otel)
	if err != nil {
		_ = otel.Shutdown(ctx)
		return nil, err
	}

	repository := triprepository.New(pool, cfg.Database.QueryTimeout)
	txManager := transaction.NewManager(pool, otel.TracerProvider.Tracer(transactionTracerName), cfg.Database.QueryTimeout)
	worker, gateway, handler, err := newPositions(cfg.Positions, logger, repository, txManager, otel)
	if err != nil {
		_ = otel.Shutdown(ctx)
		pool.Close()
		return nil, err
	}
	server, err := newServer(cfg, logger, pool, repository, txManager, otel, handler)
	if err != nil {
		gateway.Close()
		_ = otel.Shutdown(ctx)
		pool.Close()

		return nil, err
	}

	worker.Start(context.WithoutCancel(ctx))
	return &Container{Logger: logger, Server: server, pool: pool, telemetry: otel, positions: worker, push: gateway}, nil
}

func (c *Container) Shutdown(ctx context.Context) error {
	if err := c.Server.Shutdown(ctx); err != nil {
		c.positions.Abort()
		_ = c.Server.Close()
		c.push.Close()
		return fmt.Errorf("wait for in-flight requests: %w", err)
	}
	if err := c.positions.Stop(ctx); err != nil {
		c.push.Close()
		return fmt.Errorf("drain position workers: %w", err)
	}
	c.push.Close()

	c.flushTelemetry(ctx)

	closed := make(chan struct{})

	go func() {
		c.pool.Close()
		close(closed)
	}()

	select {
	case <-closed:
		return nil
	case <-ctx.Done():
		return errors.New("close database pool: shutdown budget exceeded")
	}
}

func (c *Container) flushTelemetry(ctx context.Context) {
	flushContext := ctx

	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc

		flushContext, cancel = context.WithTimeout(ctx, time.Until(deadline)/2)
		defer cancel()
	}

	if err := c.telemetry.Shutdown(flushContext); err != nil {
		c.Logger.WarnContext(ctx, "telemetry was not flushed", slog.String("error", err.Error()))
	}
}

func newPool(ctx context.Context, cfg config.DatabaseConfig, otel *telemetry.Telemetry) (*pgxpool.Pool, error) {
	queryTracer, err := postgres.NewQueryTracer(otel.TracerProvider, otel.MeterProvider)
	if err != nil {
		return nil, fmt.Errorf("create query tracer: %w", err)
	}

	connectContext, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	poolConfig, err := postgres.NewPoolConfig(cfg, queryTracer)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(connectContext, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(connectContext); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}

func newServer(
	cfg config.Config,
	logger *slog.Logger,
	pool *pgxpool.Pool,
	repository *triprepository.Repository,
	txManager *transaction.Manager,
	otel *telemetry.Telemetry,
	positionHandler *position.Handler,
) (*http.Server, error) {
	tripMetrics, err := telemetry.NewTripMetrics(otel.MeterProvider)
	if err != nil {
		return nil, fmt.Errorf("create trip metrics: %w", err)
	}

	err = telemetry.RegisterActiveTrips(otel.MeterProvider, repository.CountActive, cfg.Database.QueryTimeout, otel.ReportError)
	if err != nil {
		return nil, fmt.Errorf("register active trips gauge: %w", err)
	}

	now := time.Now

	createUsecase := createusecase.New(repository, triphash.Hasher{}, txManager, tripMetrics, logger, now, uuid.New)
	getUsecase := getusecase.New(repository)
	finishUsecase := finishusecase.New(repository, txManager, tripMetrics, logger, now)

	router, err := httpserver.NewRouter(httpserver.Options{
		Handlers: httpserver.Handlers{
			Create:    createhandler.New(createUsecase, logger),
			Get:       gethandler.New(getUsecase, logger),
			Finish:    finishhandler.New(finishUsecase, logger),
			Health:    health.New(pool, cfg.Database.QueryTimeout),
			Positions: positionHandler,
		},
		Logger:         logger,
		TracerProvider: otel.TracerProvider,
		MeterProvider:  otel.MeterProvider,
		Propagator:     otel.Propagator,
	})
	if err != nil {
		return nil, fmt.Errorf("build router: %w", err)
	}

	return httpserver.NewServer(cfg.HTTPAddr, router, cfg.HTTPTimeouts), nil
}
