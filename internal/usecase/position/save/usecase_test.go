package save_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/usecase/position/save"
)

var (
	at     = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	point  = trip.Coordinates{Latitude: 20, Longitude: 30}
)

type fixture struct {
	repository *Mockrepository
	tx         *Mocktransactions
	pending    *MockpendingRequests
	metrics    *Mockmetrics
	usecase    *save.Usecase
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	f := &fixture{
		repository: NewMockrepository(ctrl),
		tx:         NewMocktransactions(ctrl),
		pending:    NewMockpendingRequests(ctrl),
		metrics:    NewMockmetrics(ctrl),
	}
	f.usecase = save.New(f.repository, f.tx, f.pending, f.metrics, func() time.Time { return at })
	return f
}

func (f *fixture) expectTransaction(commitErr error) {
	f.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
		if err := fn(ctx); err != nil {
			return err
		}
		return commitErr
	})
}

func TestExecute(t *testing.T) {
	input := trip.PositionInput{Coordinates: point, RecordedAt: at}
	position := trip.Position{TripID: tripID, Coordinates: point, RecordedAt: at}
	saved := position
	saved.ID = 42

	tests := []struct {
		name    string
		in      trip.PositionInput
		setup   func(*fixture)
		want    trip.Position
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "save point and cancel pending request",
			in:   input,
			setup: func(f *fixture) {
				f.expectTransaction(nil)
				f.repository.EXPECT().SavePosition(gomock.Any(), position).Return(saved, nil)
				f.pending.EXPECT().Cancel(tripID)
				f.metrics.EXPECT().PositionSaved(gomock.Any())
			},
			want: saved, wantErr: assert.NoError,
		},
		{
			name: "reject a timestamp two minutes in the future",
			in:   trip.PositionInput{Coordinates: point, RecordedAt: at.Add(2 * time.Minute)},
			wantErr: func(t assert.TestingT, err error, _ ...any) bool {
				var validation *trip.ValidationError
				return assert.ErrorAs(t, err, &validation) && assert.Equal(t, "recorded_at", validation.Field)
			},
		},
		{
			name: "trip not found", in: input,
			setup: func(f *fixture) {
				f.expectTransaction(nil)
				f.repository.EXPECT().SavePosition(gomock.Any(), position).Return(trip.Position{}, trip.ErrTripNotFound)
			},
			wantErr: errorIs(trip.ErrTripNotFound),
		},
		{
			name: "trip already completed", in: input,
			setup: func(f *fixture) {
				f.expectTransaction(nil)
				f.repository.EXPECT().SavePosition(gomock.Any(), position).Return(trip.Position{}, trip.ErrTripCompleted)
			},
			wantErr: errorIs(trip.ErrTripCompleted),
		},
		{
			name: "cannot insert point", in: input,
			setup: func(f *fixture) {
				f.expectTransaction(nil)
				f.repository.EXPECT().SavePosition(gomock.Any(), position).Return(trip.Position{}, assert.AnError)
			},
			wantErr: errorIs(assert.AnError),
		},
		{
			name: "commit fails after insert", in: input,
			setup: func(f *fixture) {
				f.expectTransaction(assert.AnError)
				f.repository.EXPECT().SavePosition(gomock.Any(), position).Return(saved, nil)
			},
			wantErr: errorIs(assert.AnError),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}

			got, err := f.usecase.Execute(t.Context(), tripID, tt.in)
			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func errorIs(want error) assert.ErrorAssertionFunc {
	return func(t assert.TestingT, err error, args ...any) bool {
		return assert.ErrorIs(t, err, want, args...)
	}
}
