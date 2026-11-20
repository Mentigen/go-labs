package positions

import (
	"container/heap"
	"context"
	"sync"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mentigen/go-labs/internal/model"
)

const (
	resultQueued    = "queued"
	resultDuplicate = "duplicate"
	resultDropped   = "dropped"
	resultSuccess   = "success"
	resultError     = "error"
	resultCancelled = "cancelled"
)

type task struct {
	trip               model.StaleTrip
	origin             trace.SpanContext
	minIndex, maxIndex int
	cancelled, sending bool
}

type queue struct {
	mu             sync.Mutex
	oldest, newest taskHeap
	tasks          map[uuid.UUID]*task
	capacity       int
	changed        chan struct{}
	closed         bool
}

func newQueue(capacity int) *queue {
	return &queue{capacity: capacity, tasks: make(map[uuid.UUID]*task), newest: taskHeap{latest: true}, changed: make(chan struct{})}
}

func (q *queue) add(trip model.StaleTrip, origin trace.SpanContext) (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return resultDropped, false
	}
	if _, ok := q.tasks[trip.ID]; ok {
		return resultDuplicate, false
	}
	t := &task{trip: trip, origin: origin, minIndex: -1, maxIndex: -1}
	evicted := false
	if q.oldest.Len() == q.capacity {
		latest := q.newest.items[0]
		if compare(t, latest) >= 0 {
			return resultDropped, false
		}
		q.removeWaiting(latest)
		delete(q.tasks, latest.trip.ID)
		evicted = true
	}
	wasEmpty := q.oldest.Len() == 0
	q.tasks[trip.ID] = t
	heap.Push(&q.oldest, t)
	heap.Push(&q.newest, t)
	if wasEmpty {
		close(q.changed)
		q.changed = make(chan struct{})
	}
	return resultQueued, evicted
}

func (q *queue) removeWaiting(t *task) {
	heap.Remove(&q.oldest, t.minIndex)
	heap.Remove(&q.newest, t.maxIndex)
}

func (q *queue) take(ctx context.Context) (*task, bool) {
	for {
		if ctx.Err() != nil {
			return nil, false
		}
		q.mu.Lock()
		if q.oldest.Len() > 0 {
			t := q.oldest.items[0]
			q.removeWaiting(t)
			q.mu.Unlock()
			return t, true
		}
		if q.closed {
			q.mu.Unlock()
			return nil, false
		}
		changed := q.changed
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, false
		case <-changed:
		}
	}
}

func (q *queue) cancel(id uuid.UUID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.tasks[id]
	if !ok || t.sending {
		return false
	}
	t.cancelled = true
	if t.minIndex < 0 {
		return false
	}
	q.removeWaiting(t)
	delete(q.tasks, id)
	return true
}

func (q *queue) beginSend(t *task) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.tasks[t.trip.ID] != t || t.cancelled {
		return false
	}
	t.sending = true
	return true
}

func (q *queue) finish(t *task) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.tasks[t.trip.ID] == t {
		delete(q.tasks, t.trip.ID)
	}
}

func (q *queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	close(q.changed)
}

func (q *queue) clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.oldest.items = nil
	q.newest.items = nil
	clear(q.tasks)
}

func (q *queue) size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.oldest.Len()
}
