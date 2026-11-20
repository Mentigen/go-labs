package container

import (
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/config"
	"github.com/Mentigen/go-labs/internal/gateway/push"
	"github.com/Mentigen/go-labs/internal/http/position"
	"github.com/Mentigen/go-labs/internal/infra/postgres/transaction"
	"github.com/Mentigen/go-labs/internal/infra/telemetry"
	triprepository "github.com/Mentigen/go-labs/internal/repository/trip"
	"github.com/Mentigen/go-labs/internal/usecase/position/list"
	"github.com/Mentigen/go-labs/internal/usecase/position/save"
	"github.com/Mentigen/go-labs/internal/worker/positions"
)

func newPositions(
	cfg config.PositionsConfig,
	logger *slog.Logger,
	repo *triprepository.Repository,
	tx *transaction.Manager,
	otel *telemetry.Telemetry,
) (*positions.Pool, *push.Gateway, *position.Handler, error) {
	gateway, err := push.New(cfg.PushURL, cfg.RequestTimeout, otel.TracerProvider, otel.Propagator, uuid.New)
	if err != nil {
		return nil, nil, nil, err
	}
	metrics, err := telemetry.NewPositionMetrics(otel.MeterProvider)
	if err != nil {
		gateway.Close()
		return nil, nil, nil, err
	}
	worker := positions.New(positions.Config{
		Interval: cfg.CheckInterval, StaleAfter: cfg.StaleAfter, RequestTimeout: cfg.RequestTimeout,
		Workers: cfg.Workers, QueueSize: cfg.QueueSize, BatchSize: cfg.BatchSize,
	}, repo, gateway, metrics, logger, otel.TracerProvider.Tracer("tripgo.positions"), positions.RealClock{})
	if err := telemetry.RegisterPositionPool(otel.MeterProvider, worker.QueueSize, worker.ActiveWorkers); err != nil {
		gateway.Close()
		return nil, nil, nil, err
	}
	handler := position.New(save.New(repo, tx, worker, metrics, time.Now), list.New(repo), logger)
	return worker, gateway, handler, nil
}
