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

type visibilityHeap []visibilityEntry

// Len returns the number of entries in the heap.
func (h *visibilityHeap) Len() int { return len(*h) }

// push inserts e, sifting up to restore deadline order.
func (h *visibilityHeap) push(e visibilityEntry) {
	*h = append(*h, e)
	siftUp(*h, len(*h)-1)
}

// pop removes and returns the earliest-deadline entry. Callers must check
// length first; popping an empty heap panics.
func (h *visibilityHeap) pop() visibilityEntry {
	old := *h
	first := old[0]

	n := len(old) - 1
	if n > 0 {
		old[0] = old[n]
		*h = old[:n]
		siftDown(*h, 0)
	} else {
		*h = old[:0]
	}

	return first
}

// peek returns the earliest-deadline entry without removing it.
func (h *visibilityHeap) peek() visibilityEntry { return (*h)[0] }

func siftUp(h visibilityHeap, i int) {
	for i > 0 {
		p := (i - 1) / 2
		if !h[i].deadline.Before(h[p].deadline) {
			return
		}

		h[i], h[p] = h[p], h[i]
		i = p
	}
}

func siftDown(h visibilityHeap, i int) {
	n := len(h)

	for {
		l, r := 2*i+1, 2*i+2
		m := i

		if l < n && h[l].deadline.Before(h[m].deadline) {
			m = l
		}

		if r < n && h[r].deadline.Before(h[m].deadline) {
			m = r
		}

		if m == i {
			return
		}

		h[i], h[m] = h[m], h[i]
		i = m
	}
}
