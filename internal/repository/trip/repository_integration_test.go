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

	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.trips') IS NOT NULL`).Scan(&migrated); err != nil || !migrated {
		os.Stderr.WriteString("table trips not found: run `make migrate`\n")
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
	if err != nil {
		t.Fatalf("build trip: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := pool.Exec(ctx, `DELETE FROM trips WHERE id = $1`, created.ID); err != nil {
			t.Errorf("cleanup trip %s: %v", created.ID, err)
		}
	})

	return created
}

func TestSaveAndGetByID(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	want := newTrip(t)

	if err := e.repository.Save(ctx, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := e.repository.GetByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.ID != want.ID || got.UserID != want.UserID || got.DriverID != want.DriverID ||
		got.StartPoint != want.StartPoint || got.EndPoint != want.EndPoint ||
		got.Price != want.Price || got.Status != tripdomain.StatusActive || got.FinishedAt != nil {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestGetByIDMissingTrip(t *testing.T) {
	e := newEnv(t)

	_, err := e.repository.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, tripdomain.ErrTripNotFound) {
		t.Fatalf("error = %v, want ErrTripNotFound", err)
	}
}

func TestSecondActiveTripOfTheSameDriverIsRejected(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	first := newTrip(t)
	second := newTrip(t)
	second.DriverID = first.DriverID

	if err := e.repository.Save(ctx, first); err != nil {
		t.Fatalf("save first: %v", err)
	}

	if err := e.repository.Save(ctx, second); !errors.Is(err, tripdomain.ErrDriverBusy) {
		t.Fatalf("error = %v, want ErrDriverBusy", err)
	}
}

func TestDriverIsFreeAfterTheTripIsFinished(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	first := newTrip(t)
	second := newTrip(t)
	second.DriverID = first.DriverID

	if err := e.repository.Save(ctx, first); err != nil {
		t.Fatalf("save first: %v", err)
	}

	if err := first.Finish(time.Now().UTC()); err != nil {
		t.Fatalf("finish in domain: %v", err)
	}

	if err := e.repository.Finish(ctx, first); err != nil {
		t.Fatalf("finish in repository: %v", err)
	}

	if err := e.repository.Save(ctx, second); err != nil {
		t.Fatalf("a finished trip must free the driver: %v", err)
	}
}

func TestFinishIsConditionalOnActiveStatus(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	stored := newTrip(t)
	if err := e.repository.Save(ctx, stored); err != nil {
		t.Fatalf("save: %v", err)
	}

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := stored.Finish(finishedAt); err != nil {
		t.Fatalf("finish in domain: %v", err)
	}

	if err := e.repository.Finish(ctx, stored); err != nil {
		t.Fatalf("first finish: %v", err)
	}

	if err := e.repository.Finish(ctx, stored); !errors.Is(err, tripdomain.ErrTripCompleted) {
		t.Fatalf("second finish error = %v, want ErrTripCompleted", err)
	}

	got, err := e.repository.GetByID(ctx, stored.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.Status != tripdomain.StatusCompleted || got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt) {
		t.Errorf("unexpected stored trip: %+v", got)
	}
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
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}

	if got := historyRows(t, stored.ID); got != 1 {
		t.Errorf("history rows = %d, want 1", got)
	}

	if _, err := e.repository.GetByID(ctx, stored.ID); err != nil {
		t.Errorf("trip must be committed: %v", err)
	}
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
	if err == nil {
		t.Fatal("expected the transaction to fail on the history row with an unknown trip")
	}

	if _, err := e.repository.GetByID(ctx, stored.ID); !errors.Is(err, tripdomain.ErrTripNotFound) {
		t.Errorf("trip must be rolled back, got %v", err)
	}

	if got := historyRows(t, stored.ID); got != 0 {
		t.Errorf("history rows = %d after rollback, want 0", got)
	}
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

	if _, err := e.repository.GetByID(ctx, stored.ID); !errors.Is(err, tripdomain.ErrTripNotFound) {
		t.Errorf("trip must be rolled back after a panic, got %v", err)
	}
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
		if err != nil || !claimed {
			t.Errorf("first claim = %v, %v; want true, nil", claimed, err)
		}

		return e.repository.Save(ctx, stored)
	})
	if err != nil {
		t.Fatalf("claim and save: %v", err)
	}

	record, found, err := e.repository.GetIdempotencyKey(ctx, key, time.Hour)
	if err != nil || !found || record.RequestHash != "hash-1" || record.TripID != stored.ID {
		t.Fatalf("lookup = %+v, %v, %v", record, found, err)
	}

	claimed, err := e.repository.ClaimIdempotencyKey(ctx, key, "hash-2", stored.ID, time.Hour)
	if err != nil || claimed {
		t.Errorf("a live key must not be claimed again, got %v, %v", claimed, err)
	}

	if _, found, _ := e.repository.GetIdempotencyKey(ctx, key, -time.Hour); found {
		t.Error("a key older than the ttl must be invisible")
	}

	claimed, err = e.repository.ClaimIdempotencyKey(ctx, key, "hash-3", stored.ID, -time.Hour)
	if err != nil || !claimed {
		t.Errorf("an expired key must be taken over, got %v, %v", claimed, err)
	}
}

func TestCountActive(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	before, err := e.repository.CountActive(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if err := e.repository.Save(ctx, newTrip(t)); err != nil {
		t.Fatalf("save: %v", err)
	}

	after, err := e.repository.CountActive(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if after < before+1 {
		t.Errorf("active trips: before %d, after %d", before, after)
	}
}

func historyRows(t *testing.T, tripID uuid.UUID) int {
	t.Helper()

	var count int

	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM trip_status_history WHERE trip_id = $1`, tripID).Scan(&count)
	if err != nil {
		t.Fatalf("count history: %v", err)
	}

	return count
}

func TestConcurrentFinishCommitsExactlyOneHistoryRow(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stored := newTrip(t)
	if err := e.repository.Save(ctx, stored); err != nil {
		t.Fatal(err)
	}

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
			t.Fatal("concurrent transactions did not read the trip in time")
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
			t.Errorf("unexpected finish error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 || historyRows(t, stored.ID) != 1 {
		t.Errorf("successes = %d, conflicts = %d, want one of each and one history row", succeeded, conflicted)
	}
}

func TestTransactionBeginHasTimeoutWhenPoolIsExhausted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0
	limitedPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer limitedPool.Close()
	lease, err := limitedPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()

	manager := transaction.NewManager(limitedPool, noop.NewTracerProvider().Tracer("test"), 25*time.Millisecond)
	err = manager.Do(ctx, func(context.Context) error {
		t.Error("transaction callback must not run without a connection")
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("begin must hit its own deadline: err = %v, parent = %v", err, ctx.Err())
	}
}

func TestTransactionCanceledBeforeCommitDoesNotPersistWrites(t *testing.T) {
	e := newEnv(t)
	stored := newTrip(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := e.txManager.Do(ctx, func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("transaction must carry a deadline through commit")
		}
		if err := e.repository.Save(ctx, stored); err != nil {
			return err
		}
		if err := e.repository.AddCreatedHistory(ctx, stored.ID); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("commit error = %v, want context.Canceled", err)
	}
	if _, err := e.repository.GetByID(context.Background(), stored.ID); !errors.Is(err, tripdomain.ErrTripNotFound) {
		t.Errorf("trip must not survive a canceled commit: %v", err)
	}
	if historyRows(t, stored.ID) != 0 {
		t.Error("history must not survive a canceled commit")
	}
}
