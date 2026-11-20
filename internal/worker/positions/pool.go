package positions

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	Interval, StaleAfter, RequestTimeout time.Duration
	Workers, QueueSize, BatchSize        int
}

type Pool struct {
	cfg                    Config
	repository             repository
	gateway                gateway
	metrics                metrics
	logger                 logger
	tracer                 trace.Tracer
	clock                  Clock
	queue                  *queue
	active                 atomic.Int64
	start                  sync.Once
	scanCancel, workCancel context.CancelFunc
	done                   chan struct{}
	ctx                    context.Context
}

func New(cfg Config, repository repository, gateway gateway, metrics metrics, logger logger, tracer trace.Tracer, clock Clock) *Pool {
	return &Pool{
		cfg: cfg, repository: repository, gateway: gateway, metrics: metrics, logger: logger, tracer: tracer, clock: clock,
		queue: newQueue(cfg.QueueSize), done: make(chan struct{}),
	}
}

func (p *Pool) Start(ctx context.Context) {
	p.start.Do(func() {
		workCtx, workCancel := context.WithCancel(ctx)
		scanCtx, scanCancel := context.WithCancel(workCtx)
		p.ctx, p.workCancel, p.scanCancel = workCtx, workCancel, scanCancel
		ticks := p.clock.NewTicker(p.cfg.Interval)
		var workers sync.WaitGroup
		for range p.cfg.Workers {
			workers.Go(func() { p.work(workCtx) })
		}
		go func() {
			p.scan(scanCtx, ticks)
			ticks.Stop()
			p.queue.close()
			workers.Wait()
			p.queue.clear()
			scanCancel()
			workCancel()
			close(p.done)
		}()
	})
}

func (p *Pool) Stop(ctx context.Context) error {
	p.scanCancel()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		p.Abort()
		return ctx.Err()
	}
}

func (p *Pool) Abort() { p.workCancel() }

func (p *Pool) Cancel(id uuid.UUID) {
	if p.queue.cancel(id) {
		p.metrics.RequestFinished(p.ctx, resultCancelled)
	}
}

func (p *Pool) QueueSize() int       { return p.queue.size() }
func (p *Pool) ActiveWorkers() int64 { return p.active.Load() }
