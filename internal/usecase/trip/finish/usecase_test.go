package finish_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/usecase/trip/finish"
)

var (
	started = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	now     = started.Add(20 * time.Minute)
	tripID  = uuid.MustParse("11111111-1111-1111-1111-111111111111")

	errStorage = errors.New("storage is down")
)

type fixture struct {
	repository *MocktripRepository
	metrics    *Mockmetrics
	usecase    *finish.Usecase
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	ctrl := gomock.NewController(t)

	txManager := NewMocktxManager(ctrl)
	txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).
		AnyTimes()

	f := &fixture{
		repository: NewMocktripRepository(ctrl),
		metrics:    NewMockmetrics(ctrl),
	}

	f.usecase = finish.New(
		f.repository,
		txManager,
		f.metrics,
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		func() time.Time { return now },
	)

	return f
}

func activeTrip() tripdomain.Trip {
	return tripdomain.Trip{ID: tripID, Status: tripdomain.StatusActive, StartedAt: started, UpdatedAt: started}
}

func TestExecuteFinishesActiveTrip(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(activeTrip(), nil)
	f.repository.EXPECT().
		Finish(gomock.Any(), gomock.Cond(func(trip tripdomain.Trip) bool {
			return trip.Status == tripdomain.StatusCompleted &&
				trip.FinishedAt != nil && trip.FinishedAt.Equal(now)
		})).
		Return(nil)
	f.repository.EXPECT().AddCompletedHistory(gomock.Any(), tripID).Return(nil)
	f.metrics.EXPECT().TripCompleted(gomock.Any())

	got, err := f.usecase.Execute(context.Background(), tripID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Status != tripdomain.StatusCompleted {
		t.Errorf("status = %q, want %q", got.Status, tripdomain.StatusCompleted)
	}

	if got.FinishedAt == nil || !got.FinishedAt.Equal(now) {
		t.Errorf("finished_at = %v, want %v", got.FinishedAt, now)
	}
}

func TestExecuteCompletedTripIsRejectedByDomain(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	completed := activeTrip()
	completed.Status = tripdomain.StatusCompleted
	completed.FinishedAt = &started

	f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(completed, nil)

	_, err := f.usecase.Execute(context.Background(), tripID)
	if !errors.Is(err, tripdomain.ErrTripCompleted) {
		t.Fatalf("error = %v, want ErrTripCompleted", err)
	}
}

func TestExecuteTripNotFound(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(tripdomain.Trip{}, tripdomain.ErrTripNotFound)

	_, err := f.usecase.Execute(context.Background(), tripID)
	if !errors.Is(err, tripdomain.ErrTripNotFound) {
		t.Fatalf("error = %v, want ErrTripNotFound", err)
	}
}

func TestExecuteLostRaceAgainstConcurrentFinish(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(activeTrip(), nil)
	f.repository.EXPECT().Finish(gomock.Any(), gomock.Any()).Return(tripdomain.ErrTripCompleted)

	_, err := f.usecase.Execute(context.Background(), tripID)
	if !errors.Is(err, tripdomain.ErrTripCompleted) {
		t.Fatalf("error = %v, want ErrTripCompleted", err)
	}
}

func TestExecuteHistoryFailureFailsTheWholeOperation(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(activeTrip(), nil)
	f.repository.EXPECT().Finish(gomock.Any(), gomock.Any()).Return(nil)
	f.repository.EXPECT().AddCompletedHistory(gomock.Any(), tripID).Return(errStorage)

	_, err := f.usecase.Execute(context.Background(), tripID)
	if !errors.Is(err, errStorage) {
		t.Fatalf("error = %v, want storage error", err)
	}
}
