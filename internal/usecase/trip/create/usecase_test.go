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

	f.txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).
		AnyTimes()

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

func TestExecute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		idempotent     bool
		invalid        bool
		setup          func(*fixture)
		want           model.CreateTripResult
		wantErr        error
		wantValidation bool
		wantAnyError   bool
	}{
		{name: "created", idempotent: false, setup: func(f *fixture) {
			f.repository.EXPECT().Save(gomock.Any(), expectedTrip()).Return(nil)
			f.repository.EXPECT().AddCreatedHistory(gomock.Any(), tripID).Return(nil)
			f.metrics.EXPECT().TripCreated(gomock.Any())
			f.logger.EXPECT().InfoContext(gomock.Any(), "trip created", "trip_id", tripID.String())
		}, want: model.CreateTripResult{Trip: expectedTrip(), Created: true}},
		{name: "driver busy", idempotent: false, wantErr: tripdomain.ErrDriverBusy, setup: func(f *fixture) {
			f.repository.EXPECT().Save(gomock.Any(), gomock.Any()).Return(tripdomain.ErrDriverBusy)
		}, want: model.CreateTripResult{}},
		{name: "history failure", idempotent: false, wantErr: errStorage, setup: func(f *fixture) {
			f.repository.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
			f.repository.EXPECT().AddCreatedHistory(gomock.Any(), tripID).Return(errStorage)
		}, want: model.CreateTripResult{}},
		{name: "invalid input", idempotent: false, invalid: true, wantValidation: true, setup: func(f *fixture) {
		}, want: model.CreateTripResult{}},
		{name: "first idempotent request", idempotent: true, setup: func(f *fixture) {
			f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{}, false, nil)
			f.repository.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, "fingerprint", tripID, idempotencyTTL).Return(true, nil)
			f.repository.EXPECT().Save(gomock.Any(), expectedTrip()).Return(nil)
			f.repository.EXPECT().AddCreatedHistory(gomock.Any(), tripID).Return(nil)
			f.metrics.EXPECT().TripCreated(gomock.Any())
			f.logger.EXPECT().InfoContext(gomock.Any(), "trip created", "trip_id", tripID.String())
		}, want: model.CreateTripResult{Trip: expectedTrip(), Created: true}},
		{name: "replay completed trip", idempotent: true, setup: func(f *fixture) {
			f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{RequestHash: "fingerprint", TripID: tripID}, true, nil)
			f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(completedTrip(), nil)
		}, want: model.CreateTripResult{Trip: completedTrip()}},
		{name: "different body", idempotent: true, wantErr: tripdomain.ErrIdempotencyConflict, setup: func(f *fixture) {
			f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{RequestHash: "other", TripID: tripID}, true, nil)
		}, want: model.CreateTripResult{}},
		{name: "lost claim replay", idempotent: true, setup: func(f *fixture) {
			gomock.InOrder(
				f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{}, false, nil),
				f.repository.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, "fingerprint", tripID, idempotencyTTL).Return(false, nil),
				f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{RequestHash: "fingerprint", TripID: tripID}, true, nil),
				f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(expectedTrip(), nil),
			)
		}, want: model.CreateTripResult{Trip: expectedTrip()}},
		{name: "key disappeared", idempotent: true, wantAnyError: true, setup: func(f *fixture) {
			f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{}, false, nil).Times(2)
			f.repository.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, "fingerprint", tripID, idempotencyTTL).Return(false, nil)
		}, want: model.CreateTripResult{}},
		{name: "lookup failure", idempotent: true, wantErr: errStorage, setup: func(f *fixture) {
			f.repository.EXPECT().GetIdempotencyKey(gomock.Any(), key, idempotencyTTL).Return(model.IdempotencyRecord{}, false, errStorage)
		}, want: model.CreateTripResult{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			input := validInput()
			if tt.invalid {
				input.Price = -10
			}
			if tt.idempotent {
				input.IdempotencyKey = &key
				f.hasher.EXPECT().Params(tripdomain.NewParams{UserID: input.UserID, DriverID: input.DriverID, StartPoint: input.StartPoint, EndPoint: input.EndPoint, Price: input.Price}).Return("fingerprint")
			}
			tt.setup(f)
			got, err := f.usecase.Execute(context.Background(), input)
			switch {
			case tt.wantValidation:
				var validationError *tripdomain.ValidationError
				require.ErrorAs(t, err, &validationError)
				assert.Equal(t, "price", validationError.Field)
			case tt.wantAnyError:
				require.Error(t, err)
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			default:
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func completedTrip() tripdomain.Trip {
	trip := expectedTrip()
	trip.Status = tripdomain.StatusCompleted
	trip.FinishedAt = &now
	return trip
}
