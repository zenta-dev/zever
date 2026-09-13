package memory

import (
	"container/heap"
	"sort"
	"sync"
	"time"

	"github.com/zenta-dev/zever/queue"
)

type topicQueue struct {
	mu         sync.Mutex
	ready      []queue.Message
	delayed    []delayedEntry
	inflight   map[queue.MessageID]inflight
	visHeap    visibilityHeap
	notify     chan struct{}
	promoteCh  chan struct{}
	spaceCh    chan struct{}
	promoterOn bool
	penders    int
	quit       chan struct{}
	quitOnce   sync.Once
	done       chan struct{}
}

func newTopicQueue() *topicQueue {
	return &topicQueue{
		inflight:  make(map[queue.MessageID]inflight),
		notify:    make(chan struct{}),
		promoteCh: make(chan struct{}),
		spaceCh:   make(chan struct{}),
		quit:      make(chan struct{}),
		done:      make(chan struct{}),
	}
}

func (t *topicQueue) waitForPromote(timer *time.Timer, wait time.Duration) {
	timer.Stop()
	timer.Reset(wait)

	select {
	case <-t.quit:
		return
	case <-timer.C:
	case <-t.promoteCh:
		timer.Stop()
		return
	}
}

func (t *topicQueue) promote() {
	defer func() {
		t.mu.Lock()
		t.promoterOn = false
		done := t.done
		t.mu.Unlock()
		close(done)
	}()

	timer := time.NewTimer(time.Hour)
	defer timer.Stop()

	for {
		select {
		case <-t.quit:
			return
		default:
		}

		due, nextDue, hasMore := t.collectDue()

		if len(due) > 0 {
			t.appendDue(due)
			t.signal()
		}

		if !hasMore {
			t.mu.Lock()
			t.promoterOn = false
			t.mu.Unlock()

			return
		}

		wait := time.Until(nextDue)

		t.waitForPromote(timer, wait)
	}
}

func (t *topicQueue) collectDue() ([]delayedEntry, time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	remaining := make([]delayedEntry, 0, len(t.delayed))

	var due []delayedEntry

	var nextDue time.Time

	for _, e := range t.delayed {
		if !e.readyAt.After(now) {
			due = append(due, e)
		} else {
			remaining = append(remaining, e)

			if nextDue.IsZero() || e.readyAt.Before(nextDue) {
				nextDue = e.readyAt
			}
		}
	}

	t.delayed = remaining

	sort.Slice(due, func(i, j int) bool { return due[i].readyAt.Before(due[j].readyAt) })

	hasMore := len(remaining) > 0

	return due, nextDue, hasMore
}

func (t *topicQueue) appendDue(due []delayedEntry) {
	t.mu.Lock()
	for _, e := range due {
		t.ready = append(t.ready, e.msg)
	}
	t.mu.Unlock()
}

func (t *topicQueue) signal() {
	select {
	case t.notify <- struct{}{}:
	default:
	}
}

func (t *topicQueue) startPromoterLocked() {
	if t.promoterOn {
		return
	}

	t.promoterOn = true

	t.done = make(chan struct{})

	select {
	case <-t.quit:
		t.quit = make(chan struct{})
		t.quitOnce = sync.Once{}
	default:
	}

	go t.promote()
}

func (t *topicQueue) reclaimFromHeap(now time.Time) bool {
	reclaimed := false

	for t.visHeap.Len() > 0 {
		top := t.visHeap[0]
		if now.Before(top.deadline) {
			break
		}

		raw := heap.Pop(&t.visHeap)
		entry, ok := raw.(*visibilityEntry)
		if !ok {
			continue
		}

		inf, ok := t.inflight[entry.id]
		if !ok {
			continue
		}

		if !inf.deadline.Equal(entry.deadline) {
			if now.After(inf.deadline) {
				t.reclaimEntry(entry.id, inf)

				reclaimed = true
			}

			continue
		}

		if now.After(inf.deadline) {
			t.reclaimEntry(entry.id, inf)

			reclaimed = true
		} else {
			heap.Push(&t.visHeap, entry)

			break
		}
	}

	return reclaimed
}

func (t *topicQueue) reclaimFromMap(now time.Time) bool {
	if t.visHeap.Len() != 0 || len(t.inflight) == 0 {
		return false
	}

	reclaimed := false

	for id, inf := range t.inflight {
		if now.After(inf.deadline) {
			t.reclaimEntry(id, inf)

			reclaimed = true
		}
	}

	return reclaimed
}

func (t *topicQueue) reclaimExpired(_ time.Duration) {
	now := time.Now()

	t.mu.Lock()
	reclaimed := t.reclaimFromHeap(now) || t.reclaimFromMap(now)
	t.mu.Unlock()

	if reclaimed {
		t.signal()
	}
}

func (t *topicQueue) tryPopReady(vis time.Duration) (queue.Message, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.ready) == 0 {
		return queue.Message{}, false
	}

	msg := t.ready[0]
	t.ready = t.ready[1:]
	deadline := time.Now().Add(vis)
	t.inflight[msg.ID] = inflight{
		msg:      msg,
		deadline: deadline,
	}

	heap.Push(&t.visHeap, &visibilityEntry{id: msg.ID, deadline: deadline})

	return msg.Clone(), true
}

func (t *topicQueue) reclaimEntry(id queue.MessageID, inf inflight) {
	inf.msg = inf.msg.Clone()
	inf.msg.Attempt++
	t.ready = append(t.ready, inf.msg)
	delete(t.inflight, id)
}

func (t *topicQueue) signalSpace() {
	select {
	case t.spaceCh <- struct{}{}:
	default:
	}
}
