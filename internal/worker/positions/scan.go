package positions

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mentigen/go-labs/internal/model"
)

func (p *Pool) scan(ctx context.Context, ticks Ticker) {
	p.logger.InfoContext(ctx, "position scanner started")
	defer p.logger.InfoContext(ctx, "position scanner stopped")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks.C():
			if ctx.Err() != nil {
				return
			}
			p.scanOnce(ctx)
		}
	}
}

func (p *Pool) scanOnce(ctx context.Context) {
	ctx, span := p.tracer.Start(ctx, "positions.scan")
	defer span.End()
	rowsRead := 0
	defer func() { p.metrics.RowsRead(ctx, rowsRead) }()
	var cutoff time.Time
	var cursor *model.PositionCursor
	for ctx.Err() == nil {
		page, err := p.repository.StaleTrips(ctx, p.cfg.StaleAfter, cutoff, cursor, p.cfg.BatchSize)
		if err != nil {
			if ctx.Err() == nil {
				p.logger.WarnContext(ctx, "position scan failed", "error", err.Error())
			}
			span.RecordError(err)
			span.SetStatus(codes.Error, "position scan failed")
			return
		}
		cutoff = page.Cutoff
		rowsRead += len(page.Trips)
		for _, trip := range page.Trips {
			if ctx.Err() != nil {
				return
			}
			result, evicted := p.queue.add(trip, trace.SpanContextFromContext(ctx))
			p.metrics.StaleCheck(ctx, result)
			if evicted {
				p.metrics.StaleCheck(ctx, resultDropped)
			}
			if result == resultDropped || evicted {
				p.logger.WarnContext(ctx, "position queue full, task dropped")
				return
			}
		}
		if len(page.Trips) < p.cfg.BatchSize {
			return
		}
		last := page.Trips[len(page.Trips)-1]
		cursor = &model.PositionCursor{PositionAt: last.PositionAt, ID: last.ID}
	}
}
