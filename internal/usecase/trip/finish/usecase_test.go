package finish_test

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
	"github.com/Mentigen/go-labs/internal/usecase/trip/finish"
)

var (
	started = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	now     = started.Add(20 * time.Minute)
	tripID  = uuid.MustParse("11111111-1111-1111-1111-111111111111")

	errStorage = errors.New("storage is down")
)

type fixture struct {
	logger     *Mocklogger
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
		})

	f := &fixture{
		logger:     NewMocklogger(ctrl),
		repository: NewMocktripRepository(ctrl),
		metrics:    NewMockmetrics(ctrl),
	}

	f.usecase = finish.New(
		f.repository,
		txManager,
		f.metrics,
		f.logger,
		func() time.Time { return now },
	)

	return f
}

func activeTrip() tripdomain.Trip {
	return tripdomain.Trip{ID: tripID, Status: tripdomain.StatusActive, StartedAt: started, UpdatedAt: started}
}

func TestExecute(t *testing.T) {
	t.Parallel()
	completed := activeTrip()
	completed.Status = tripdomain.StatusCompleted
	completed.FinishedAt = &now
	completed.UpdatedAt = now
	tests := []struct {
		name    string
		setup   func(*fixture)
		want    tripdomain.Trip
		wantErr error
	}{
		{
			name: "finish active trip",
			setup: func(f *fixture) {
				f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(activeTrip(), nil)
				f.repository.EXPECT().Finish(gomock.Any(), completed).Return(nil)
				f.repository.EXPECT().AddCompletedHistory(gomock.Any(), tripID).Return(nil)
				f.metrics.EXPECT().TripCompleted(gomock.Any())
				f.logger.EXPECT().InfoContext(gomock.Any(), "trip completed", "trip_id", tripID.String())
			},
			want: completed,
		},
		{
			name: "already completed",
			setup: func(f *fixture) {
				f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(completed, nil)
			},
			wantErr: tripdomain.ErrTripCompleted,
		},
		{
			name: "trip not found",
			setup: func(f *fixture) {
				f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(tripdomain.Trip{}, tripdomain.ErrTripNotFound)
			},
			wantErr: tripdomain.ErrTripNotFound,
		},
		{
			name: "another request finished the trip first",
			setup: func(f *fixture) {
				f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(activeTrip(), nil)
				f.repository.EXPECT().Finish(gomock.Any(), completed).Return(tripdomain.ErrTripCompleted)
			},
			wantErr: tripdomain.ErrTripCompleted,
		},
		{
			name: "cannot save history",
			setup: func(f *fixture) {
				f.repository.EXPECT().GetByID(gomock.Any(), tripID).Return(activeTrip(), nil)
				f.repository.EXPECT().Finish(gomock.Any(), completed).Return(nil)
				f.repository.EXPECT().AddCompletedHistory(gomock.Any(), tripID).Return(errStorage)
			},
			wantErr: errStorage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			tt.setup(f)

			got, err := f.usecase.Execute(context.Background(), tripID)
			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, got)
		})
	}
}
