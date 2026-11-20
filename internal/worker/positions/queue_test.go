package positions

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mentigen/go-labs/internal/model"
)

var testTime = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func candidate(n int) model.StaleTrip {
	id := uuid.UUID{}
	id[14], id[15] = byte((n/256)&255), byte(n&255)
	return model.StaleTrip{ID: id, DriverID: id, PositionAt: testTime.Add(time.Duration(n) * time.Second), UpdatedAt: testTime}
}

func TestQueueAdd(t *testing.T) {
	first, second, third, fourth := candidate(1), candidate(2), candidate(3), candidate(4)
	tied := second
	tied.PositionAt = first.PositionAt
	tests := []struct {
		name         string
		waiting      []model.StaleTrip
		in           model.StaleTrip
		wantResult   string
		wantEviction bool
		wantOrder    []model.StaleTrip
	}{
		{
			name: "oldest first across insertions", waiting: []model.StaleTrip{third, first}, in: second,
			wantResult: resultQueued, wantOrder: []model.StaleTrip{first, second, third},
		},
		{
			name: "older trip replaces newest", waiting: []model.StaleTrip{fourth, third, second}, in: first,
			wantResult: resultQueued, wantEviction: true, wantOrder: []model.StaleTrip{first, second, third},
		},
		{
			name: "full queue drops newer trip", waiting: []model.StaleTrip{first, second, third}, in: fourth,
			wantResult: resultDropped, wantOrder: []model.StaleTrip{first, second, third},
		},
		{
			name: "duplicate keeps its place", waiting: []model.StaleTrip{first, second}, in: second,
			wantResult: resultDuplicate, wantOrder: []model.StaleTrip{first, second},
		},
		{
			name: "equal timestamps ordered by id", waiting: []model.StaleTrip{tied}, in: first,
			wantResult: resultQueued, wantOrder: []model.StaleTrip{first, tied},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newQueue(3)
			for _, trip := range tt.waiting {
				q.add(trip, trace.SpanContext{})
			}

			result, evicted := q.add(tt.in, trace.SpanContext{})
			assert.Equal(t, tt.wantResult, result)
			assert.Equal(t, tt.wantEviction, evicted)

			q.close()
			require.Equal(t, len(tt.wantOrder), q.size())
			for _, want := range tt.wantOrder {
				task, ok := q.take(t.Context())
				require.True(t, ok)
				assert.Equal(t, want, task.trip)
			}
		})
	}
}

func TestQueueCancel(t *testing.T) {
	trip := candidate(1)
	tests := []struct {
		name        string
		setup       func(*testing.T, *queue)
		wantRemoved bool
		wantRequeue string
	}{
		{name: "waiting trip can be queued again", wantRemoved: true, wantRequeue: resultQueued},
		{
			name: "request already being sent stays in flight",
			setup: func(t *testing.T, q *queue) {
				task, ok := q.take(t.Context())
				require.True(t, ok)
				require.True(t, q.beginSend(task))
			},
			wantRequeue: resultDuplicate,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newQueue(1)
			q.add(trip, trace.SpanContext{})
			if tt.setup != nil {
				tt.setup(t, q)
			}

			assert.Equal(t, tt.wantRemoved, q.cancel(trip.ID))
			result, _ := q.add(trip, trace.SpanContext{})
			assert.Equal(t, tt.wantRequeue, result)
		})
	}
}

func TestFinishingPreviousTaskKeepsRequeuedTrip(t *testing.T) {
	q := newQueue(1)
	trip := candidate(1)
	q.add(trip, trace.SpanContext{})
	previous, ok := q.take(t.Context())
	require.True(t, ok)
	q.finish(previous)

	result, _ := q.add(trip, trace.SpanContext{})
	require.Equal(t, resultQueued, result)
	q.finish(previous)

	result, _ = q.add(trip, trace.SpanContext{})
	assert.Equal(t, resultDuplicate, result)
}
