package create_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestCreateTrip(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*fixture)
		want    model.CreateTripResult
		wantErr error
	}{
		{
			name: "new trip",
			setup: func(f *fixture) {
				f.repository.EXPECT().Save(gomock.Any(), expectedTrip()).Return(nil)
				f.repository.EXPECT().AddCreatedHistory(gomock.Any(), tripID).Return(nil)
				f.expectCreationEvents()
			},
			want: model.CreateTripResult{Trip: expectedTrip(), Created: true},
		},
		{
			name: "driver already has an active trip",
			setup: func(f *fixture) {
				f.repository.EXPECT().Save(gomock.Any(), expectedTrip()).Return(tripdomain.ErrDriverBusy)
			},
			wantErr: tripdomain.ErrDriverBusy,
		},
		{
			name: "cannot save history",
			setup: func(f *fixture) {
				f.repository.EXPECT().Save(gomock.Any(), expectedTrip()).Return(nil)
				f.repository.EXPECT().AddCreatedHistory(gomock.Any(), tripID).Return(errStorage)
			},
			wantErr: errStorage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.expectTransaction()
			tt.setup(f)

			got, err := f.usecase.Execute(context.Background(), validInput())
			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreateRejectsNegativePrice(t *testing.T) {
	f := newFixture(t)
	input := validInput()
	input.Price = -10

	got, err := f.usecase.Execute(context.Background(), input)
	var validation *tripdomain.ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Equal(t, "price", validation.Field)
	assert.Empty(t, got)
}

func TestCreateWithIdempotencyKey(t *testing.T) {
	input := validInput()
	input.IdempotencyKey = &key
	params := tripdomain.NewParams{
		UserID: input.UserID, DriverID: input.DriverID, StartPoint: input.StartPoint,
		EndPoint: input.EndPoint, Price: input.Price,
	}
	existing := tripdomain.Trip{
		ID:     uuid.MustParse("55555555-5555-5555-5555-555555555555"),
		Status: tripdomain.StatusCompleted, FinishedAt: &now,
	}
	record := model.IdempotencyRecord{RequestHash: "fingerprint", TripID: existing.ID}
	tests := []struct {
		name       string
		setup      func(*fixture)
		want       model.CreateTripResult
		checkError assert.ErrorAssertionFunc
	}{
		{
			name: "first request creates a trip",
			setup: func(f *fixture) {
				f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{}, false, nil)
				f.expectTransaction()
				claim := f.repository.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, record.RequestHash, tripID, idempotencyTTL).Return(true, nil)
				f.repository.EXPECT().Save(gomock.Any(), expectedTrip()).After(claim).Return(nil)
				f.repository.EXPECT().AddCreatedHistory(gomock.Any(), tripID).Return(nil)
				f.expectCreationEvents()
			},
			want:       model.CreateTripResult{Trip: expectedTrip(), Created: true},
			checkError: assert.NoError,
		},
		{
			name: "retry returns the existing trip even after it finished",
			setup: func(f *fixture) {
				f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(record, true, nil)
				f.repository.EXPECT().GetByID(gomock.Any(), existing.ID).Return(existing, nil)
			},
			want:       model.CreateTripResult{Trip: existing},
			checkError: assert.NoError,
		},
		{
			name: "same key with a different body",
			setup: func(f *fixture) {
				other := model.IdempotencyRecord{TripID: tripID, RequestHash: "other"}
				f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(other, true, nil)
			},
			checkError: func(t assert.TestingT, err error, _ ...any) bool {
				return assert.ErrorIs(t, err, tripdomain.ErrIdempotencyConflict)
			},
		},
		{
			name: "cannot look up the key",
			setup: func(f *fixture) {
				f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{}, false, errStorage)
			},
			checkError: func(t assert.TestingT, err error, _ ...any) bool {
				return assert.ErrorIs(t, err, errStorage)
			},
		},
		{
			name: "concurrent request claims the key first",
			setup: func(f *fixture) {
				f.expectLostClaim()
				f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(record, true, nil)
				f.repository.EXPECT().GetByID(gomock.Any(), existing.ID).Return(existing, nil)
			},
			want:       model.CreateTripResult{Trip: existing},
			checkError: assert.NoError,
		},
		{
			name: "key disappeared after a concurrent claim",
			setup: func(f *fixture) {
				f.expectLostClaim()
				f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{}, false, nil)
			},
			checkError: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.hasher.EXPECT().Params(params).Return(record.RequestHash)
			tt.setup(f)

			got, err := f.usecase.Execute(context.Background(), input)
			tt.checkError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func (f *fixture) expectLostClaim() {
	f.expectTransaction()
	gomock.InOrder(
		f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{}, false, nil),
		f.repository.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, "fingerprint", tripID, idempotencyTTL).Return(false, nil),
	)
}

type fixture struct {
	hasher     *MockrequestHasher
	logger     *Mocklogger
	repository *MocktripRepository
	txManager  *MocktxManager
	metrics    *Mockmetrics
	usecase    *create.Usecase
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	ctrl := gomock.NewController(t)

	f := &fixture{
		hasher:     NewMockrequestHasher(ctrl),
		logger:     NewMocklogger(ctrl),
		repository: NewMocktripRepository(ctrl),
		txManager:  NewMocktxManager(ctrl),
		metrics:    NewMockmetrics(ctrl),
	}

	f.usecase = create.New(
		f.repository,
		f.hasher,
		f.txManager,
		f.metrics,
		f.logger,
		func() time.Time { return now },
		func() uuid.UUID { return tripID },
	)

	return f
}

func (f *fixture) expectTransaction() {
	f.txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		})
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
	in := validInput()
	return tripdomain.Trip{
		ID: tripID, UserID: in.UserID, DriverID: in.DriverID, Price: in.Price,
		StartPoint: in.StartPoint, EndPoint: in.EndPoint, Status: tripdomain.StatusActive,
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func (f *fixture) expectCreationEvents() {
	f.metrics.EXPECT().TripCreated(gomock.Any())
	f.logger.EXPECT().InfoContext(gomock.Any(), "trip created", "trip_id", tripID.String())
}
