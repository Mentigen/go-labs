//go:build integration

package trip_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/infra/postgres/transaction"
	tripstore "github.com/Mentigen/go-labs/internal/repository/trip"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		os.Stderr.WriteString("DATABASE_URL is required: run `tripgoctl environment start` and `make migrate`\n")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error

	pool, err = pgxpool.New(ctx, url)
	if err != nil {
		os.Stderr.WriteString("connect to database: " + err.Error() + "\n")
		return 1
	}

	defer pool.Close()

	var migrated bool

	if err := pool.QueryRow(ctx, `SELECT to_regclass('trip_positions') IS NOT NULL`).Scan(&migrated); err != nil || !migrated {
		os.Stderr.WriteString("table trip_positions not found: run `make migrate`\n")
		return 1
	}

	return m.Run()
}

type env struct {
	repository *tripstore.Repository
	txManager  *transaction.Manager
}

func newEnv(t *testing.T) env {
	t.Helper()

	return env{
		repository: tripstore.New(pool, 3*time.Second),
		txManager:  transaction.NewManager(pool, noop.NewTracerProvider().Tracer("test"), 3*time.Second),
	}
}

func newTrip(t *testing.T) tripdomain.Trip {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Microsecond)

	created, err := tripdomain.New(uuid.New(), now, tripdomain.NewParams{
		UserID:     uuid.New(),
		DriverID:   uuid.New(),
		StartPoint: tripdomain.Coordinates{Latitude: 55.75, Longitude: 37.61},
		EndPoint:   tripdomain.Coordinates{Latitude: 55.80, Longitude: 37.70},
		Price:      500,
	})
	require.NoError(t, err, "build trip: %v", err)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := pool.Exec(ctx, `DELETE FROM trips WHERE id = $1`, created.ID)
		assert.NoError(t, err)
	})

	return created
}

func TestSaveAndGetByID(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	want := newTrip(t)

	require.NoError(t, e.repository.Save(ctx, want))

	got, err := e.repository.GetByID(ctx, want.ID)
	require.NoError(t, err, "get: %v", err)

	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.UserID, got.UserID)
	assert.Equal(t, want.DriverID, got.DriverID)
	assert.Equal(t, want.StartPoint, got.StartPoint)
	assert.Equal(t, want.EndPoint, got.EndPoint)
	assert.Equal(t, want.Price, got.Price)
	assert.Equal(t, tripdomain.StatusActive, got.Status)
	assert.Nil(t, got.FinishedAt)
}

func TestGetByIDMissingTrip(t *testing.T) {
	e := newEnv(t)

	_, err := e.repository.GetByID(context.Background(), uuid.New())
	require.ErrorIs(t, err, tripdomain.ErrTripNotFound, "error = %v, want ErrTripNotFound", err)
}

func TestSecondActiveTripOfTheSameDriverIsRejected(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	first := newTrip(t)
	second := newTrip(t)
	second.DriverID = first.DriverID

	require.NoError(t, e.repository.Save(ctx, first))

	require.ErrorIs(t, e.repository.Save(ctx, second), tripdomain.ErrDriverBusy)
}

func TestDriverIsFreeAfterTheTripIsFinished(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	first := newTrip(t)
	second := newTrip(t)
	second.DriverID = first.DriverID

	require.NoError(t, e.repository.Save(ctx, first))

	require.NoError(t, first.Finish(time.Now().UTC()))

	require.NoError(t, e.repository.Finish(ctx, first))

	require.NoError(t, e.repository.Save(ctx, second))
}

func TestFinishIsConditionalOnActiveStatus(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	stored := newTrip(t)
	require.NoError(t, e.repository.Save(ctx, stored))

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, stored.Finish(finishedAt))

	require.NoError(t, e.repository.Finish(ctx, stored))

	require.ErrorIs(t, e.repository.Finish(ctx, stored), tripdomain.ErrTripCompleted)

	got, err := e.repository.GetByID(ctx, stored.ID)
	require.NoError(t, err, "get: %v", err)

	assert.Equal(t, tripdomain.StatusCompleted, got.Status)
	require.NotNil(t, got.FinishedAt)
	assert.True(t, got.FinishedAt.Equal(finishedAt))
}

func TestTransactionCommitsBothTables(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	stored := newTrip(t)

	err := e.txManager.Do(ctx, func(ctx context.Context) error {
		if err := e.repository.Save(ctx, stored); err != nil {
			return err
		}

		return e.repository.AddCreatedHistory(ctx, stored.ID)
	})
	require.NoError(t, err, "transaction: %v", err)

	assert.EqualValues(t, 1, historyRows(t, stored.ID))

	_, err = e.repository.GetByID(ctx, stored.ID)
	assert.NoError(t, err)
}

func TestTransactionRollsBackBothTablesOnError(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	stored := newTrip(t)

	err := e.txManager.Do(ctx, func(ctx context.Context) error {
		if err := e.repository.Save(ctx, stored); err != nil {
			return err
		}

		if err := e.repository.AddCreatedHistory(ctx, stored.ID); err != nil {
			return err
		}

		return e.repository.AddCreatedHistory(ctx, uuid.New())
	})
	require.Error(t, err, "expected the transaction to fail on the history row with an unknown trip")

	_, err = e.repository.GetByID(ctx, stored.ID)
	assert.ErrorIs(t, err, tripdomain.ErrTripNotFound)

	assert.EqualValues(t, 0, historyRows(t, stored.ID))
}

func TestTransactionRollsBackOnPanic(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	stored := newTrip(t)

	func() {
		defer func() { _ = recover() }()

		_ = e.txManager.Do(ctx, func(ctx context.Context) error {
			if err := e.repository.Save(ctx, stored); err != nil {
				return err
			}

			panic("boom")
		})
	}()

	_, err := e.repository.GetByID(ctx, stored.ID)
	assert.ErrorIs(t, err, tripdomain.ErrTripNotFound)
}

func TestIdempotencyKeyLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	stored := newTrip(t)
	key := uuid.New()

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM idempotency_keys WHERE key = $1`, key)
	})

	err := e.txManager.Do(ctx, func(ctx context.Context) error {
		claimed, err := e.repository.ClaimIdempotencyKey(ctx, key, "hash-1", stored.ID, time.Hour)
		assert.NoError(t, err)
		assert.True(t, claimed)

		return e.repository.Save(ctx, stored)
	})
	require.NoError(t, err, "claim and save: %v", err)

	record, found, err := e.repository.GetIdempotencyKey(ctx, key, time.Hour)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "hash-1", record.RequestHash)
	require.Equal(t, stored.ID, record.TripID)

	claimed, err := e.repository.ClaimIdempotencyKey(ctx, key, "hash-2", stored.ID, time.Hour)
	assert.NoError(t, err)
	assert.False(t, claimed)

	_, found, err = e.repository.GetIdempotencyKey(ctx, key, -time.Hour)
	require.NoError(t, err)
	assert.False(t, found)

	claimed, err = e.repository.ClaimIdempotencyKey(ctx, key, "hash-3", stored.ID, -time.Hour)
	assert.NoError(t, err)
	assert.True(t, claimed)
}

func TestCountActive(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	before, err := e.repository.CountActive(ctx)
	require.NoError(t, err, "count: %v", err)

	require.NoError(t, e.repository.Save(ctx, newTrip(t)))

	after, err := e.repository.CountActive(ctx)
	require.NoError(t, err, "count: %v", err)

	assert.GreaterOrEqual(t, after, before+1)
}

func historyRows(t *testing.T, tripID uuid.UUID) int {
	t.Helper()

	var count int

	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM trip_status_history WHERE trip_id = $1`, tripID).Scan(&count)
	require.NoError(t, err, "count history: %v", err)

	return count
}

func TestConcurrentFinishCommitsExactlyOneHistoryRow(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stored := newTrip(t)
	require.NoError(t, e.repository.Save(ctx, stored))

	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- e.txManager.Do(ctx, func(ctx context.Context) error {
				current, err := e.repository.GetByID(ctx, stored.ID)
				ready <- struct{}{}
				if err != nil {
					return err
				}
				select {
				case <-start:
				case <-ctx.Done():
					return ctx.Err()
				}
				if err := current.Finish(time.Now().UTC()); err != nil {
					return err
				}
				if err := e.repository.Finish(ctx, current); err != nil {
					return err
				}
				return e.repository.AddCompletedHistory(ctx, stored.ID)
			})
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case <-ctx.Done():
			require.Fail(t, "concurrent transactions did not read the trip in time")
		}
	}
	close(start)
	var succeeded, conflicted int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, tripdomain.ErrTripCompleted):
			conflicted++
		default:
			assert.NoError(t, err, "unexpected finish error")
		}
	}
	assert.EqualValues(t, 1, succeeded)
	assert.EqualValues(t, 1, conflicted)
	assert.EqualValues(t, 1, historyRows(t, stored.ID))
}

func TestTransactionBeginHasTimeoutWhenPoolIsExhausted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	require.NoError(t, err)
	cfg.MaxConns = 1
	cfg.MinConns = 0
	limitedPool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer limitedPool.Close()
	lease, err := limitedPool.Acquire(ctx)
	require.NoError(t, err)
	defer lease.Release()

	manager := transaction.NewManager(limitedPool, noop.NewTracerProvider().Tracer("test"), 25*time.Millisecond)
	err = manager.Do(ctx, func(context.Context) error {
		assert.Fail(t, "transaction callback must not run without a connection")
		return nil
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, ctx.Err())
}

func TestTransactionCanceledBeforeCommitDoesNotPersistWrites(t *testing.T) {
	e := newEnv(t)
	stored := newTrip(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := e.txManager.Do(ctx, func(ctx context.Context) error {
		_, ok := ctx.Deadline()
		assert.True(t, ok)
		if err := e.repository.Save(ctx, stored); err != nil {
			return err
		}
		if err := e.repository.AddCreatedHistory(ctx, stored.ID); err != nil {
			return err
		}
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled, "commit error = %v, want context.Canceled", err)
	_, err = e.repository.GetByID(context.Background(), stored.ID)
	assert.ErrorIs(t, err, tripdomain.ErrTripNotFound)
	assert.EqualValues(t, 0, historyRows(t, stored.ID))
}
