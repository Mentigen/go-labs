package positions

import "bytes"

type taskHeap struct {
	items  []*task
	latest bool
}

func (h taskHeap) Len() int { return len(h.items) }
func (h taskHeap) Less(i, j int) bool {
	order := compare(h.items[i], h.items[j])
	if h.latest {
		return order > 0
	}
	return order < 0
}

func compare(a, b *task) int {
	if n := a.trip.PositionAt.Compare(b.trip.PositionAt); n != 0 {
		return n
	}
	return bytes.Compare(a.trip.ID[:], b.trip.ID[:])
}

func (h taskHeap) setIndex(t *task, i int) {
	if h.latest {
		t.maxIndex = i
	} else {
		t.minIndex = i
	}
}

func (h taskHeap) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
	h.setIndex(h.items[i], i)
	h.setIndex(h.items[j], j)
}

func (h *taskHeap) Push(value any) {
	t := value.(*task)
	h.setIndex(t, len(h.items))
	h.items = append(h.items, t)
}

func (h *taskHeap) Pop() any {
	i := len(h.items) - 1
	t := h.items[i]
	h.items[i] = nil
	h.items = h.items[:i]
	h.setIndex(t, -1)
	return t
}
