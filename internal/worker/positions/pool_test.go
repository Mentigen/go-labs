package positions

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/Mentigen/go-labs/internal/model"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type fixture struct {
	pool        *Pool
	repo        *Mockrepository
	gateway     *Mockgateway
	metrics     *Mockmetrics
	ticks       chan time.Time
	scanStopped chan struct{}
}

func newFixture(t *testing.T, workers, capacity int) *fixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo, gateway, metrics, logger := NewMockrepository(ctrl), NewMockgateway(ctrl), NewMockmetrics(ctrl), NewMocklogger(ctrl)
	clock, ticker := NewMockClock(ctrl), NewMockTicker(ctrl)
	ticks := make(chan time.Time, 2)
	clock.EXPECT().Now().Return(testTime).AnyTimes()
	clock.EXPECT().NewTicker(time.Second).Return(ticker).MaxTimes(1)
	ticker.EXPECT().C().Return((<-chan time.Time)(ticks)).AnyTimes()
	scanStopped := make(chan struct{})
	ticker.EXPECT().Stop().Do(func() { close(scanStopped) }).MaxTimes(1)
	logger.EXPECT().InfoContext(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().WarnContext(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().WarnContext(gomock.Any(), gomock.Any()).AnyTimes()
	metrics.EXPECT().RequestDuration(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	p := New(Config{Interval: time.Second, StaleAfter: time.Minute, RequestTimeout: time.Second, Workers: workers, QueueSize: capacity, BatchSize: 100},
		repo, gateway, metrics, logger, noop.NewTracerProvider().Tracer("test"), clock)
	return &fixture{pool: p, repo: repo, gateway: gateway, metrics: metrics, ticks: ticks, scanStopped: scanStopped}
}

func start(t *testing.T, p *Pool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		assert.NoError(t, p.Stop(stopCtx))
		cancel()
	})
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for test event")
	}
	var zero T
	return zero
}

func expectPage(f *fixture, first, last int) {
	trips := make([]model.StaleTrip, 0, last-first+1)
	for n := first; n <= last; n++ {
		trips = append(trips, candidate(n))
	}
	var cutoff time.Time
	var cursor *model.PositionCursor
	if first > 1 {
		previous := candidate(first - 1)
		cutoff = testTime
		cursor = &model.PositionCursor{PositionAt: previous.PositionAt, ID: previous.ID}
	}
	f.repo.EXPECT().StaleTrips(gomock.Any(), time.Minute, cutoff, cursor, f.pool.cfg.BatchSize).
		Return(model.StalePage{Trips: trips, Cutoff: testTime}, nil)
}

func TestScanOnce(t *testing.T) {
	tests := []struct {
		name                              string
		capacity                          int
		setup                             func(*fixture)
		wantRows, wantQueued, wantDropped int
	}{
		{
			name: "read all pages", capacity: 300,
			setup: func(f *fixture) {
				expectPage(f, 1, 100)
				expectPage(f, 101, 200)
				expectPage(f, 201, 250)
			},
			wantRows: 250, wantQueued: 250,
		},
		{
			name: "full queue drops a new trip and stops scan", capacity: 50,
			setup:    func(f *fixture) { expectPage(f, 1, 100) },
			wantRows: 100, wantQueued: 50, wantDropped: 1,
		},
		{
			name: "older trip evicts a waiting trip and stops scan", capacity: 1,
			setup: func(f *fixture) {
				f.pool.cfg.BatchSize = 1
				f.pool.queue.add(candidate(3), trace.SpanContext{})
				expectPage(f, 1, 1)
			},
			wantRows: 1, wantQueued: 1, wantDropped: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, 1, tt.capacity)
			tt.setup(f)
			f.metrics.EXPECT().RowsRead(gomock.Any(), tt.wantRows)
			f.metrics.EXPECT().StaleCheck(gomock.Any(), resultQueued).Times(tt.wantQueued)
			f.metrics.EXPECT().StaleCheck(gomock.Any(), resultDropped).Times(tt.wantDropped)

			f.pool.scanOnce(t.Context())

			require.Equal(t, tt.wantQueued, f.pool.QueueSize())
			first, ok := f.pool.queue.take(t.Context())
			require.True(t, ok)
			assert.Equal(t, candidate(1), first.trip)
		})
	}
}

func TestWorkersAreBoundedAndDrain(t *testing.T) {
	f := newFixture(t, 4, 20)
	entered := make(chan struct{}, 8)
	finished := make(chan string, 8)
	release := make(chan struct{})
	f.repo.EXPECT().PositionRequestNeeded(gomock.Any(), gomock.Any()).Return(true, nil).Times(8)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
		entered <- struct{}{}
		<-release
		return nil
	}).Times(8)
	f.metrics.EXPECT().RequestFinished(gomock.Any(), gomock.Any()).Do(func(_ context.Context, result string) { finished <- result }).Times(8)
	for n := 1; n <= 8; n++ {
		f.pool.queue.add(candidate(n), trace.SpanContext{})
	}
	start(t, f.pool)
	for range 4 {
		receive(t, entered)
	}
	assert.EqualValues(t, 4, f.pool.ActiveWorkers())
	assert.Equal(t, 4, f.pool.QueueSize())
	stopped := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { stopped <- f.pool.Stop(ctx) }()
	receive(t, f.scanStopped)
	close(release)
	for range 8 {
		assert.Equal(t, resultSuccess, receive(t, finished))
	}
	require.NoError(t, receive(t, stopped))
	assert.Zero(t, f.pool.ActiveWorkers())
	assert.Zero(t, f.pool.QueueSize())
}

func TestRepeatedTicksDoNotDuplicateInFlightRequest(t *testing.T) {
	f := newFixture(t, 1, 3)
	entered, release := make(chan struct{}), make(chan struct{})
	scanned := make(chan struct{}, 2)
	f.repo.EXPECT().StaleTrips(gomock.Any(), time.Minute, time.Time{}, (*model.PositionCursor)(nil), 100).
		Return(model.StalePage{Trips: []model.StaleTrip{candidate(1)}, Cutoff: testTime}, nil).Times(2)
	f.metrics.EXPECT().RowsRead(gomock.Any(), 1).Do(func(context.Context, int) { scanned <- struct{}{} }).Times(2)
	f.metrics.EXPECT().StaleCheck(gomock.Any(), resultQueued)
	f.metrics.EXPECT().StaleCheck(gomock.Any(), resultDuplicate)
	f.repo.EXPECT().PositionRequestNeeded(gomock.Any(), candidate(1)).Return(true, nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), candidate(1).ID, candidate(1).DriverID).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
			close(entered)
			<-release
			return nil
		})
	f.metrics.EXPECT().RequestFinished(gomock.Any(), resultSuccess)
	start(t, f.pool)
	defer close(release)
	f.ticks <- testTime
	receive(t, entered)
	receive(t, scanned)
	f.ticks <- testTime.Add(time.Second)
	receive(t, scanned)
	assert.Zero(t, f.pool.QueueSize())
	assert.EqualValues(t, 1, f.pool.ActiveWorkers())
}

func TestPoolCancel(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*fixture, chan struct{}, <-chan struct{})
		cancelTrip  int
		wantSuccess int
	}{
		{
			name: "coordinate arrives while waiting", cancelTrip: 2, wantSuccess: 1,
			setup: func(f *fixture, entered chan struct{}, release <-chan struct{}) {
				f.pool.queue.add(candidate(1), trace.SpanContext{})
				f.pool.queue.add(candidate(2), trace.SpanContext{})
				f.repo.EXPECT().PositionRequestNeeded(gomock.Any(), candidate(1)).Return(true, nil)
				f.gateway.EXPECT().RequestPosition(gomock.Any(), candidate(1).ID, candidate(1).DriverID).
					DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
						close(entered)
						<-release
						return nil
					})
			},
		},
		{
			name: "coordinate arrives during database check", cancelTrip: 1,
			setup: func(f *fixture, entered chan struct{}, release <-chan struct{}) {
				f.pool.queue.add(candidate(1), trace.SpanContext{})
				f.repo.EXPECT().PositionRequestNeeded(gomock.Any(), candidate(1)).
					DoAndReturn(func(context.Context, model.StaleTrip) (bool, error) {
						close(entered)
						<-release
						return true, nil
					})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, 1, 2)
			entered, release := make(chan struct{}), make(chan struct{})
			tt.setup(f, entered, release)
			f.metrics.EXPECT().RequestFinished(gomock.Any(), resultCancelled)
			f.metrics.EXPECT().RequestFinished(gomock.Any(), resultSuccess).Times(tt.wantSuccess)
			start(t, f.pool)
			defer close(release)
			receive(t, entered)

			f.pool.Cancel(candidate(tt.cancelTrip).ID)
			f.pool.Cancel(candidate(tt.cancelTrip).ID)

			assert.Zero(t, f.pool.QueueSize())
		})
	}
}

func TestWorkerContinuesAfterError(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fixture, model.StaleTrip)
		want  string
	}{
		{
			name: "trip changed",
			setup: func(f *fixture, trip model.StaleTrip) {
				f.repo.EXPECT().PositionRequestNeeded(gomock.Any(), trip).Return(false, nil)
			},
			want: resultCancelled,
		},
		{
			name: "database error",
			setup: func(f *fixture, trip model.StaleTrip) {
				f.repo.EXPECT().PositionRequestNeeded(gomock.Any(), trip).Return(false, assert.AnError)
			},
			want: resultError,
		},
		{
			name: "push error",
			setup: func(f *fixture, trip model.StaleTrip) {
				f.repo.EXPECT().PositionRequestNeeded(gomock.Any(), trip).Return(true, nil)
				f.gateway.EXPECT().RequestPosition(gomock.Any(), trip.ID, trip.DriverID).Return(assert.AnError)
			},
			want: resultError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, 1, 2)
			first, next := candidate(1), candidate(2)
			tt.setup(f, first)
			f.repo.EXPECT().PositionRequestNeeded(gomock.Any(), next).Return(true, nil)
			f.gateway.EXPECT().RequestPosition(gomock.Any(), next.ID, next.DriverID).Return(nil)
			finished := make(chan string, 2)
			f.metrics.EXPECT().RequestFinished(gomock.Any(), gomock.Any()).
				Do(func(_ context.Context, result string) { finished <- result }).Times(2)
			f.pool.queue.add(first, trace.SpanContext{})
			f.pool.queue.add(next, trace.SpanContext{})

			start(t, f.pool)

			assert.Equal(t, tt.want, receive(t, finished))
			assert.Equal(t, resultSuccess, receive(t, finished))
		})
	}
}

func TestStopDeadlineCancelsInFlightRequest(t *testing.T) {
	f := newFixture(t, 1, 2)
	entered := make(chan struct{})
	f.repo.EXPECT().PositionRequestNeeded(gomock.Any(), gomock.Any()).Return(true, nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	f.metrics.EXPECT().RequestFinished(gomock.Any(), resultError)
	f.pool.queue.add(candidate(1), trace.SpanContext{})
	start(t, f.pool)
	receive(t, entered)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, f.pool.Stop(ctx), context.Canceled)
	receive(t, f.pool.done)
	assert.Zero(t, f.pool.ActiveWorkers())
}

func TestParentCancellationStopsBlockedScan(t *testing.T) {
	f := newFixture(t, 2, 3)
	entered := make(chan struct{})
	f.repo.EXPECT().StaleTrips(gomock.Any(), time.Minute, time.Time{}, (*model.PositionCursor)(nil), 100).
		DoAndReturn(func(ctx context.Context, _ time.Duration, _ time.Time, _ *model.PositionCursor, _ int) (model.StalePage, error) {
			close(entered)
			<-ctx.Done()
			return model.StalePage{}, ctx.Err()
		})
	f.metrics.EXPECT().RowsRead(gomock.Any(), 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.pool.Start(ctx)
	f.ticks <- testTime
	receive(t, entered)
	cancel()
	receive(t, f.pool.done)
	assert.Zero(t, f.pool.ActiveWorkers())
	assert.Zero(t, f.pool.QueueSize())
}

func TestWorkerSpanLinksToScan(t *testing.T) {
	f := newFixture(t, 1, 3)
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { assert.NoError(t, provider.Shutdown(context.Background())) })
	f.pool.tracer = provider.Tracer("test")
	f.repo.EXPECT().StaleTrips(gomock.Any(), time.Minute, time.Time{}, (*model.PositionCursor)(nil), 100).
		Return(model.StalePage{Trips: []model.StaleTrip{candidate(1)}, Cutoff: testTime}, nil)
	f.metrics.EXPECT().RowsRead(gomock.Any(), 1)
	f.metrics.EXPECT().StaleCheck(gomock.Any(), resultQueued)
	f.repo.EXPECT().PositionRequestNeeded(gomock.Any(), candidate(1)).Return(true, nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), candidate(1).ID, candidate(1).DriverID).Return(nil)
	f.metrics.EXPECT().RequestFinished(gomock.Any(), resultSuccess)
	f.pool.scanOnce(context.Background())
	task, ok := f.pool.queue.take(context.Background())
	require.True(t, ok)
	f.pool.process(context.Background(), task)
	spans := recorder.Ended()
	require.Len(t, spans, 2)
	scan, worker := spans[0], spans[1]
	assert.Equal(t, "positions.scan", scan.Name())
	assert.Equal(t, "positions.request", worker.Name())
	assert.False(t, worker.Parent().IsValid())
	assert.NotEqual(t, scan.SpanContext().TraceID(), worker.SpanContext().TraceID())
	require.Len(t, worker.Links(), 1)
	assert.Equal(t, scan.SpanContext(), worker.Links()[0].SpanContext)
}
