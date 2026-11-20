//go:build integration

package trip_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/model"
)

func (e env) savePosition(ctx context.Context, tripID uuid.UUID, at time.Time, afterSave error) (tripdomain.Position, error) {
	var saved tripdomain.Position
	err := e.txManager.Do(ctx, func(ctx context.Context) error {
		var err error
		saved, err = e.repository.SavePosition(ctx, tripdomain.Position{TripID: tripID, RecordedAt: at})
		if err != nil {
			return err
		}
		return afterSave
	})
	return saved, err
}

func TestSavePosition(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Microsecond)
	older, newer := at.Add(-time.Minute), at.Add(time.Minute)
	rollback := errors.New("abort transaction")
	tests := []struct {
		name       string
		recordedAt time.Time
		afterSave  error
		wantLast   time.Time
		wantPoints int
	}{
		{name: "newer point advances last position", recordedAt: newer, wantLast: newer, wantPoints: 2},
		{name: "late point keeps last position", recordedAt: older, wantLast: at, wantPoints: 2},
		{name: "rollback discards point and timestamp", recordedAt: newer, afterSave: rollback, wantLast: at, wantPoints: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			e := newEnv(t)
			trip := newTrip(t)
			require.NoError(t, e.repository.Save(ctx, trip))
			_, err := e.savePosition(ctx, trip.ID, at, nil)
			require.NoError(t, err)

			_, err = e.savePosition(ctx, trip.ID, tt.recordedAt, tt.afterSave)
			require.ErrorIs(t, err, tt.afterSave)

			got, err := e.repository.GetByID(ctx, trip.ID)
			require.NoError(t, err)
			require.NotNil(t, got.LastPositionAt)
			assert.True(t, got.LastPositionAt.Equal(tt.wantLast))

			points, err := e.repository.ListPositions(ctx, trip.ID)
			require.NoError(t, err)
			assert.Len(t, points, tt.wantPoints)
		})
	}
}

func TestListPositions(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Microsecond)
	tests := []struct {
		name       string
		tripExists bool
		recordedAt []time.Time
		wantOrder  []int
		wantErr    error
	}{
		{name: "missing trip", wantErr: tripdomain.ErrTripNotFound},
		{name: "empty route", tripExists: true},
		{
			name:       "sort by stored time and break ties by id",
			tripExists: true,
			recordedAt: []time.Time{at.Add(789 * time.Nanosecond), at, at.Add(-time.Minute)},
			wantOrder:  []int{2, 0, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			e := newEnv(t)
			trip := newTrip(t)
			if tt.tripExists {
				require.NoError(t, e.repository.Save(ctx, trip))
			}
			var saved []tripdomain.Position
			for _, recorded := range tt.recordedAt {
				point, err := e.savePosition(ctx, trip.ID, recorded, nil)
				require.NoError(t, err)
				assert.True(t, point.RecordedAt.Equal(recorded.Truncate(time.Microsecond)))
				saved = append(saved, point)
			}

			points, err := e.repository.ListPositions(ctx, trip.ID)
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantErr != nil {
				assert.Nil(t, points)
				return
			}
			want := make([]tripdomain.Position, 0, len(tt.wantOrder))
			for _, i := range tt.wantOrder {
				want = append(want, saved[i])
			}
			assert.Equal(t, want, points)
		})
	}
}

func TestSavePositionDuringFinish(t *testing.T) {
	rollback := errors.New("roll back finish")
	tests := []struct {
		name        string
		finishErr   error
		wantSaveErr error
		wantPoints  int
	}{
		{name: "finish committed", wantSaveErr: tripdomain.ErrTripCompleted},
		{name: "finish rolled back", finishErr: rollback, wantPoints: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			trip := newTrip(t)
			require.NoError(t, e.repository.Save(ctx, trip))

			saved := make(chan error, 1)
			err := e.txManager.Do(ctx, func(txCtx context.Context) error {
				locked, err := e.repository.GetByIDForUpdate(txCtx, trip.ID)
				if err != nil {
					return err
				}
				_, err = pool.Exec(ctx, "SELECT id FROM trips WHERE id=$1 FOR UPDATE NOWAIT", trip.ID)
				var lockErr *pgconn.PgError
				if assert.ErrorAs(t, err, &lockErr) {
					assert.Equal(t, "55P03", lockErr.Code)
				}
				if err := locked.Finish(time.Now()); err != nil {
					return err
				}
				if err := e.repository.Finish(txCtx, locked); err != nil {
					return err
				}
				go func() {
					_, err := e.savePosition(ctx, trip.ID, time.Now(), nil)
					saved <- err
				}()
				return tt.finishErr
			})
			require.ErrorIs(t, err, tt.finishErr)
			require.ErrorIs(t, <-saved, tt.wantSaveErr)

			points, err := e.repository.ListPositions(ctx, trip.ID)
			require.NoError(t, err)
			assert.Len(t, points, tt.wantPoints)
		})
	}
}

func TestStaleTrips(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	stale := now.Add(-time.Hour)
	tests := []struct {
		name       string
		startedAt  time.Time
		recordedAt time.Time
		completed  bool
		want       bool
	}{
		{name: "old start without points", startedAt: stale, want: true},
		{name: "recent start without points", startedAt: now},
		{name: "recent point", startedAt: stale, recordedAt: now},
		{name: "stale point", startedAt: stale, recordedAt: now.Add(-2 * time.Minute), want: true},
		{name: "completed trip", startedAt: stale, completed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			e := newEnv(t)
			trip := newTrip(t)
			trip.StartedAt = tt.startedAt
			if tt.completed {
				require.NoError(t, trip.Finish(now))
			}
			require.NoError(t, e.repository.Save(ctx, trip))
			if !tt.recordedAt.IsZero() {
				_, err := e.savePosition(ctx, trip.ID, tt.recordedAt, nil)
				require.NoError(t, err)
			}

			page, err := e.repository.StaleTrips(ctx, time.Minute, time.Time{}, nil, 10000)
			require.NoError(t, err)
			included := slices.ContainsFunc(page.Trips, func(row model.StaleTrip) bool { return row.ID == trip.ID })
			assert.Equal(t, tt.want, included)
		})
	}
}

func TestPositionRequestNeeded(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	tests := []struct {
		name   string
		change func(*testing.T, env, tripdomain.Trip)
		want   bool
	}{
		{name: "trip unchanged since scan", want: true},
		{
			name: "older point arrived after scan",
			change: func(t *testing.T, e env, trip tripdomain.Trip) {
				_, err := e.savePosition(t.Context(), trip.ID, at.Add(-time.Hour), nil)
				require.NoError(t, err)
			},
		},
		{
			name: "trip finished after scan",
			change: func(t *testing.T, e env, trip tripdomain.Trip) {
				require.NoError(t, trip.Finish(time.Now()))
				require.NoError(t, e.repository.Finish(t.Context(), trip))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			ctx := t.Context()
			trip := newTrip(t)
			require.NoError(t, e.repository.Save(ctx, trip))
			_, err := e.savePosition(ctx, trip.ID, at, nil)
			require.NoError(t, err)
			page, err := e.repository.StaleTrips(ctx, time.Minute, time.Time{}, nil, 10000)
			require.NoError(t, err)
			index := slices.IndexFunc(page.Trips, func(row model.StaleTrip) bool { return row.ID == trip.ID })
			require.NotEqual(t, -1, index)
			if tt.change != nil {
				tt.change(t, e, trip)
			}

			needed, err := e.repository.PositionRequestNeeded(ctx, page.Trips[index])
			require.NoError(t, err)
			assert.Equal(t, tt.want, needed)
		})
	}
}

func TestStalePaginationEqualTimes(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	at := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := make([]uuid.UUID, 0, 5)
	for range 5 {
		trip := newTrip(t)
		trip.StartedAt = at
		require.NoError(t, e.repository.Save(ctx, trip))
		ids = append(ids, trip.ID)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

	tests := []struct {
		name  string
		after uuid.UUID
		want  []uuid.UUID
	}{
		{name: "first full page", after: uuid.Nil, want: ids[:2]},
		{name: "next full page", after: ids[1], want: ids[2:4]},
		{name: "final partial page", after: ids[3], want: ids[4:]},
		{name: "end of range", after: ids[4]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursor := &model.PositionCursor{PositionAt: at, ID: tt.after}
			page, err := e.repository.StaleTrips(ctx, time.Minute, at.Add(time.Microsecond), cursor, 2)
			require.NoError(t, err)
			var got []uuid.UUID
			for _, row := range page.Trips {
				got = append(got, row.ID)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
