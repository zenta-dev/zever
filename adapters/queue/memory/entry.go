package memory

import (
	"time"

	"github.com/zenta-dev/zever/core/queue"
)

type delayedEntry struct {
	msg     queue.Message
	readyAt time.Time
}

type visibilityEntry struct {
	id       queue.MessageID
	deadline time.Time
}

type visibilityHeap []*visibilityEntry //nolint:recvcheck // heap.Interface requires mixed receivers

func (h visibilityHeap) Len() int           { return len(h) }
func (h visibilityHeap) Less(i, j int) bool { return h[i].deadline.Before(h[j].deadline) }
func (h visibilityHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *visibilityHeap) Push(x any) {
	if e, ok := x.(*visibilityEntry); ok {
		*h = append(*h, e)
	}
}

func (h *visibilityHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]

	return x
}
