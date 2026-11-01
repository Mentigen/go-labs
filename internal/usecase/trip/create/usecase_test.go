package create_test

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
	"github.com/Mentigen/go-labs/internal/model"
	"github.com/Mentigen/go-labs/internal/usecase/trip/create"
)

const idempotencyTTL = 24 * time.Hour

var (
	now      = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	tripID   = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	userID   = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	driverID = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	key      = uuid.MustParse("44444444-4444-4444-4444-444444444444")

	errStorage = errors.New("storage is down")
)

type fixture struct {
	repository *MocktripRepository
	txManager  *MocktxManager
	metrics    *Mockmetrics
	usecase    *create.Usecase
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	ctrl := gomock.NewController(t)

	f := &fixture{
		repository: NewMocktripRepository(ctrl),
		txManager:  NewMocktxManager(ctrl),
		metrics:    NewMockmetrics(ctrl),
	}

	f.txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).
		AnyTimes()

	f.usecase = create.New(
		f.repository,
		f.txManager,
		f.metrics,
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		func() time.Time { return now },
		func() uuid.UUID { return tripID },
	)

	return f
}

func validInput() model.CreateTripInput {
	return model.CreateTripInput{
		UserID:     userID,
		DriverID:   driverID,
		StartPoint: tripdomain.Coordinates{Latitude: 55.75, Longitude: 37.61},
		EndPoint:   tripdomain.Coordinates{Latitude: 55.80, Longitude: 37.70},
		Price:      500,
	}
}

func expectedTrip() tripdomain.Trip {
	return tripdomain.Trip{
		ID:         tripID,
		UserID:     userID,
		DriverID:   driverID,
		StartPoint: tripdomain.Coordinates{Latitude: 55.75, Longitude: 37.61},
		EndPoint:   tripdomain.Coordinates{Latitude: 55.80, Longitude: 37.70},
		Price:      500,
		Status:     tripdomain.StatusActive,
		StartedAt:  now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func TestExecuteCreatesTrip(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ctx := context.Background()

	f.repository.EXPECT().Save(gomock.Any(), expectedTrip()).Return(nil)
	f.repository.EXPECT().AddCreatedHistory(gomock.Any(), tripID).Return(nil)
	f.metrics.EXPECT().TripCreated(gomock.Any())

	result, err := f.usecase.Execute(ctx, validInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Created {
		t.Error("created = false, want true")
	}

	if result.Trip != expectedTrip() {
		t.Errorf("trip = %+v, want %+v", result.Trip, expectedTrip())
	}
}

func TestExecuteDriverBusy(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	f.repository.EXPECT().Save(gomock.Any(), gomock.Any()).Return(tripdomain.ErrDriverBusy)

	_, err := f.usecase.Execute(context.Background(), validInput())
	if !errors.Is(err, tripdomain.ErrDriverBusy) {
		t.Fatalf("error = %v, want ErrDriverBusy", err)
	}
}

func TestExecuteHistoryFailureFailsTheWholeOperation(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	f.repository.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	f.repository.EXPECT().AddCreatedHistory(gomock.Any(), tripID).Return(errStorage)

	_, err := f.usecase.Execute(context.Background(), validInput())
	if !errors.Is(err, errStorage) {
		t.Fatalf("error = %v, want storage error", err)
	}
}

func TestExecuteRejectsInvalidInputWithoutTouchingStorage(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	input := validInput()
	input.Price = -10

	_, err := f.usecase.Execute(context.Background(), input)

	var validationError *tripdomain.ValidationError
	if !errors.As(err, &validationError) || validationError.Field != "price" {
		t.Fatalf("error = %v, want validation error for price", err)
	}
}

func TestExecuteIdempotentFirstRequestCreatesTrip(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	input := validInput()
	input.IdempotencyKey = &key

	f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{}, false, nil)
	f.repository.EXPECT().
		ClaimIdempotencyKey(gomock.Any(), key, gomock.Not(gomock.Eq("")), tripID, idempotencyTTL).
		Return(true, nil)
	f.repository.EXPECT().Save(gomock.Any(), expectedTrip()).Return(nil)
	f.repository.EXPECT().AddCreatedHistory(gomock.Any(), tripID).Return(nil)
	f.metrics.EXPECT().TripCreated(gomock.Any())

	result, err := f.usecase.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Created {
		t.Error("created = false, want true")
	}
}

func TestExecuteIdempotentReplayReturnsStoredTrip(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	input := validInput()
	input.IdempotencyKey = &key

	var claimedHash string

	f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).
		Return(model.IdempotencyRecord{}, false, nil)
	f.repository.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, gomock.Any(), tripID, idempotencyTTL).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, hash string, _ uuid.UUID, _ time.Duration) (bool, error) {
			claimedHash = hash
			return true, nil
		})
	f.repository.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	f.repository.EXPECT().AddCreatedHistory(gomock.Any(), tripID).Return(nil)
	f.metrics.EXPECT().TripCreated(gomock.Any())

	if _, err := f.usecase.Execute(context.Background(), input); err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	stored := expectedTrip()
	stored.Status = tripdomain.StatusCompleted

	f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).
		DoAndReturn(func(context.Context, uuid.UUID, time.Duration) (model.IdempotencyRecord, bool, error) {
			return model.IdempotencyRecord{RequestHash: claimedHash, TripID: tripID}, true, nil
		})
	f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(stored, nil)

	result, err := f.usecase.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	if result.Created {
		t.Error("created = true on replay, want false")
	}

	if result.Trip.Status != tripdomain.StatusCompleted {
		t.Errorf("replay must return the current state, got status %q", result.Trip.Status)
	}
}

func TestExecuteIdempotentKeyReusedWithDifferentBody(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	input := validInput()
	input.IdempotencyKey = &key

	f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).
		Return(model.IdempotencyRecord{RequestHash: "hash-of-another-body", TripID: tripID}, true, nil)

	_, err := f.usecase.Execute(context.Background(), input)
	if !errors.Is(err, tripdomain.ErrIdempotencyConflict) {
		t.Fatalf("error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestExecuteIdempotentLostClaimFallsBackToReplay(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	input := validInput()
	input.IdempotencyKey = &key

	var claimedHash string

	gomock.InOrder(
		f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).
			Return(model.IdempotencyRecord{}, false, nil),
		f.repository.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, gomock.Any(), tripID, idempotencyTTL).
			DoAndReturn(func(_ context.Context, _ uuid.UUID, hash string, _ uuid.UUID, _ time.Duration) (bool, error) {
				claimedHash = hash
				return false, nil
			}),
		f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).
			DoAndReturn(func(context.Context, uuid.UUID, time.Duration) (model.IdempotencyRecord, bool, error) {
				return model.IdempotencyRecord{RequestHash: claimedHash, TripID: tripID}, true, nil
			}),
		f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(expectedTrip(), nil),
	)

	result, err := f.usecase.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Created {
		t.Error("created = true, want false: another request owns the key")
	}
}

func TestExecuteIdempotentKeyDisappearedAfterLostClaim(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	input := validInput()
	input.IdempotencyKey = &key

	f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).
		Return(model.IdempotencyRecord{}, false, nil).Times(2)
	f.repository.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, gomock.Any(), tripID, idempotencyTTL).
		Return(false, nil)

	if _, err := f.usecase.Execute(context.Background(), input); err == nil {
		t.Fatal("expected an error when the key vanished between claim and lookup")
	}
}

func TestExecuteIdempotentLookupFailure(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	input := validInput()
	input.IdempotencyKey = &key

	f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).
		Return(model.IdempotencyRecord{}, false, errStorage)

	_, err := f.usecase.Execute(context.Background(), input)
	if !errors.Is(err, errStorage) {
		t.Fatalf("error = %v, want storage error", err)
	}
}
