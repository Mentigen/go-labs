package positions

import (
	"context"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func (p *Pool) work(ctx context.Context) {
	p.logger.InfoContext(ctx, "position worker started")
	defer p.logger.InfoContext(ctx, "position worker stopped")
	for {
		task, ok := p.queue.take(ctx)
		if !ok {
			return
		}
		p.process(ctx, task)
	}
}

func (p *Pool) process(ctx context.Context, t *task) {
	defer p.queue.finish(t)
	p.active.Add(1)
	defer p.active.Add(-1)
	ctx, cancel := context.WithTimeout(ctx, p.cfg.RequestTimeout)
	defer cancel()
	ctx, span := p.tracer.Start(ctx, "positions.request", trace.WithNewRoot(), trace.WithLinks(trace.Link{SpanContext: t.origin}))
	defer span.End()
	result := resultError
	defer func() { p.metrics.RequestFinished(ctx, result) }()
	needed, err := p.repository.PositionRequestNeeded(ctx, t.trip)
	if err != nil {
		p.failed(ctx, span, err)
		return
	}
	if !needed || !p.queue.beginSend(t) {
		result = resultCancelled
		return
	}
	started := p.clock.Now()
	err = p.gateway.RequestPosition(ctx, t.trip.ID, t.trip.DriverID)
	if err == nil {
		result = resultSuccess
	} else {
		p.failed(ctx, span, err)
	}
	p.metrics.RequestDuration(ctx, result, p.clock.Now().Sub(started))
}

func (p *Pool) failed(ctx context.Context, span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, "position request failed")
	p.logger.WarnContext(ctx, "position request failed", "error", err.Error())
}
