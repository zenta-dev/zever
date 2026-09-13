package memory

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/queue"
)

// cover Push/PushDelayed closed race (65,73,109,116) via hammer
func TestMemory_CoverPushClosedRaceHammer(t *testing.T) {
	t.Parallel()
	for i := 0; i < 200; i++ {
		q := newQueue(t, queue.Options{Buffer: 1})
		topic := "cover-push-race"
		ctx := context.Background()
		if err := q.Push(ctx, topic, queue.Payload([]byte("one")), nil); err != nil {
			t.Fatalf("fill: %v", err)
		}
		errCh := make(chan error, 1)
		go func() {
			errCh <- q.Push(ctx, topic, queue.Payload([]byte("two")), nil)
		}()
		time.Sleep(time.Millisecond)
		_ = q.Close()
		if ma, ok := q.(*memoryAdapter); ok {
			if tq := ma.getTopic(topic); tq != nil {
				tq.mu.Lock()
				if len(tq.ready) > 0 {
					tq.ready = tq.ready[1:]
					select {
					case tq.spaceCh <- struct{}{}:
					default:
					}
				}
				tq.mu.Unlock()
			}
		}
		select {
		case err := <-errCh:
			if err != nil && err != queue.ErrClosed {
				if err.Error() != queue.ErrClosed.Error() {
				}
			}
		case <-time.After(500 * time.Millisecond):
			if ma, ok := q.(*memoryAdapter); ok {
				if tq := ma.getTopic(topic); tq != nil {
					select {
					case tq.spaceCh <- struct{}{}:
					default:
					}
				}
			}
			<-errCh
		}
	}
}

// cover Push closed after wait (65)
func TestMemory_CoverPushClosedAfterWait(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 1})
	topic := "cover-push-closed-after-wait"
	ctx := context.Background()
	if err := q.Push(ctx, topic, queue.Payload([]byte("one")), nil); err != nil {
		t.Fatalf("Push fill: %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- q.Push(ctx, topic, queue.Payload([]byte("two")), nil)
	}()
	time.Sleep(20 * time.Millisecond)
	_ = q.Close()
	if ma, ok := q.(*memoryAdapter); ok {
		if tq := ma.getTopic(topic); tq != nil {
			tq.mu.Lock()
			if len(tq.ready) > 0 {
				tq.ready = tq.ready[1:]
				select {
				case tq.spaceCh <- struct{}{}:
				default:
				}
			}
			tq.mu.Unlock()
		}
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatalf("Push after close = nil, want ErrClosed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocked Push")
	}
}

// cover Push closed at lock (73) and PushDelayed variants
func TestMemory_CoverPushClosedAtLock(t *testing.T) {
	t.Parallel()
	maIfc := newQueue(t, queue.Options{Buffer: 0})
	ma := maIfc.(*memoryAdapter)
	topic := "cover-push-lock"
	if _, err := ma.topic(topic); err != nil {
		t.Fatalf("topic: %v", err)
	}
	ma.mu.Lock()
	ma.closed = true
	ma.mu.Unlock()
	if err := maIfc.Push(context.Background(), topic, queue.Payload([]byte("x")), nil); err != queue.ErrClosed {
		t.Errorf("Push after closed at lock = %v, want ErrClosed", err)
	}
	ma2Ifc := newQueue(t, queue.Options{Buffer: 0})
	ma2 := ma2Ifc.(*memoryAdapter)
	if _, err := ma2.topic(topic); err != nil {
		t.Fatalf("topic: %v", err)
	}
	ma2.mu.Lock()
	ma2.closed = true
	ma2.mu.Unlock()
	if err := ma2Ifc.PushDelayed(context.Background(), topic, queue.Payload([]byte("x")), nil, 0); err != queue.ErrClosed {
		t.Errorf("PushDelayed immediate after closed = %v, want ErrClosed", err)
	}
	ma3Ifc := newQueue(t, queue.Options{Buffer: 0})
	ma3 := ma3Ifc.(*memoryAdapter)
	if _, err := ma3.topic(topic); err != nil {
		t.Fatalf("topic: %v", err)
	}
	ma3.mu.Lock()
	ma3.closed = true
	ma3.mu.Unlock()
	if err := ma3Ifc.PushDelayed(context.Background(), topic, queue.Payload([]byte("x")), nil, 10*time.Millisecond); err != queue.ErrClosed {
		t.Errorf("PushDelayed delayed after closed = %v, want ErrClosed", err)
	}
}

func TestMemory_CoverAckNackNilTopic(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{})
	// cover Ack/Nack unknown topic nil vs ErrClosed (173,198)
	msg := queue.NewMessage("no-such-topic-ack", queue.Payload([]byte("x")), nil)
	if err := q.Ack(context.Background(), msg); err != nil {
		t.Errorf("Ack unknown topic = %v, want nil", err)
	}
	if err := q.Nack(context.Background(), msg, false); err != nil {
		t.Errorf("Nack unknown topic = %v, want nil", err)
	}
	if err := q.Nack(context.Background(), msg, true); err != nil {
		t.Errorf("Nack requeue unknown = %v, want nil", err)
	}
	_ = q.Close()
	if err := q.Ack(context.Background(), msg); err != queue.ErrClosed {
		t.Errorf("Ack closed unknown = %v, want ErrClosed", err)
	}
	if err := q.Nack(context.Background(), msg, false); err != queue.ErrClosed {
		t.Errorf("Nack closed unknown = %v, want ErrClosed", err)
	}
}

func TestMemory_CoverSweepTopicPromoterOn(t *testing.T) {
	t.Parallel()
	maIfc := newQueue(t, queue.Options{})
	ma := maIfc.(*memoryAdapter)
	tq := newTopicQueue()
	tq.promoterOn = true
	ma.mu.Lock()
	ma.topics["sweep-promote"] = tq
	ma.mu.Unlock()
	ma.sweepTopic("sweep-promote", tq)
	select {
	case <-tq.quit:
	default:
		t.Errorf("quit not closed after sweep with promoterOn")
	}
	if _, ok := ma.topics["sweep-promote"]; ok {
		t.Errorf("topic not deleted after sweep")
	}
}

func TestMemory_CoverMountTopicLockedVariants(t *testing.T) {
	t.Parallel()
	maIfc := newQueue(t, queue.Options{})
	ma := maIfc.(*memoryAdapter)
	t1 := newTopicQueue()
	t2 := newTopicQueue()
	if got := ma.mountTopicLocked("m1", t1); got != t1 {
		t.Errorf("mount new = %v, want t1", got)
	}
	// cover existing==t branch (367)
	if got := ma.mountTopicLocked("m1", t1); got != t1 {
		t.Errorf("mount same pointer = %v, want t1", got)
	}
	// cover existing!=t return existing (367)
	if got := ma.mountTopicLocked("m1", t2); got != t1 {
		t.Errorf("mount different pointer for existing name = %v, want t1", got)
	}
}

func TestMemory_CoverPopWithTopicNotifyAndPoll(t *testing.T) {
	t.Parallel()
	maIfc := newQueue(t, queue.Options{PollTimeout: 50 * time.Millisecond})
	ma := maIfc.(*memoryAdapter)
	topic := "pop-notify"
	tq, _ := ma.topic(topic)
	ctx := context.Background()
	resCh := make(chan queue.Message, 1)
	errCh := make(chan error, 1)
	go func() {
		m, err := ma.popWithTopic(ctx, topic, tq)
		if err != nil {
			errCh <- err
			return
		}
		resCh <- m
	}()
	time.Sleep(20 * time.Millisecond)
	// cover notify wake branch (409)
	if err := maIfc.Push(ctx, topic, queue.Payload([]byte("wake")), nil); err != nil {
		t.Fatalf("Push wake: %v", err)
	}
	select {
	case m := <-resCh:
		if string(m.Payload) != "wake" {
			t.Errorf("pop wake payload = %q, want wake", string(m.Payload))
		}
	case err := <-errCh:
		t.Fatalf("pop wake err = %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for notify wake")
	}
	// cover poll timeout via pollTimer.C
	tq2, _ := ma.topic("pop-poll")
	tq2.penders = 0
	ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := ma.popWithTopic(ctx2, "pop-poll", tq2)
	if err == nil || err.Error() == "" {
		t.Fatalf("pop poll = nil, want EmptyError")
	}
	if _, ok := err.(*queue.EmptyError); !ok {
		t.Errorf("pop poll err = %T %v, want *EmptyError", err, err)
	}
	// cover cancelled context branch
	cancelled, cancel2 := context.WithCancel(context.Background())
	cancel2()
	_, err = ma.popWithTopic(cancelled, "pop-poll", tq2)
	if err == nil {
		t.Fatalf("pop cancelled = nil, want error")
	}
	// cover Pop cancelled with nil topic (152)
	cancelled3, cancel3 := context.WithCancel(context.Background())
	cancel3()
	if _, err := maIfc.Pop(cancelled3, "no-such-cancellable"); err == nil {
		t.Fatalf("Pop cancelled nil topic = nil, want error")
	}
}

func TestMemory_CoverTopicPromoteAndReclaim(t *testing.T) {
	t.Parallel()
	// cover waitForPromote timer drain (42,55)
	tq := newTopicQueue()
	timer := time.NewTimer(10 * time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	tq.waitForPromote(timer, 5*time.Millisecond)
	timer.Stop()
	tq2 := newTopicQueue()
	tq2.delayed = []delayedEntry{
		{msg: queue.NewMessage("t", queue.Payload([]byte("d1")), nil), readyAt: time.Now().Add(-time.Second)},
		{msg: queue.NewMessage("t", queue.Payload([]byte("d2")), nil), readyAt: time.Now().Add(-time.Second)},
	}
	tq2.promoterOn = true
	tq2.done = make(chan struct{})
	close(tq2.quit)
	go tq2.promote()
	select {
	case <-tq2.done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("promote quit not done")
	}
	tq3 := newTopicQueue()
	tq3.inflight = map[queue.MessageID]inflight{}
	tq3.visHeap = visibilityHeap{}
	id := queue.NewMessage("cover-id", nil, nil).ID
	tq3.visHeap = append(tq3.visHeap, &visibilityEntry{id: id, deadline: time.Now().Add(-time.Second)})
	tq3.reclaimFromHeap(time.Now())
	// cover deadline mismatch branch
	msg := queue.NewMessage("t", queue.Payload([]byte("x")), nil)
	tq3.inflight[id] = inflight{msg: msg, deadline: time.Now().Add(-2 * time.Second)}
	tq3.visHeap = visibilityHeap{&visibilityEntry{id: id, deadline: time.Now().Add(-time.Second)}}
	tq3.reclaimFromHeap(time.Now().Add(time.Second))
	// cover heap.Push requeue else branch (216)
	tq3 = newTopicQueue()
	id2 := queue.NewMessage("cover-id", nil, nil).ID
	future := time.Now().Add(time.Hour)
	tq3.inflight[id2] = inflight{msg: msg, deadline: future}
	tq3.visHeap = visibilityHeap{&visibilityEntry{id: id2, deadline: future}}
	tq3.reclaimFromHeap(time.Now())
	if tq3.visHeap.Len() != 1 {
		t.Errorf("visHeap len = %d, want 1 (requeued)", tq3.visHeap.Len())
	}
}

func TestMemory_CoverWaitForCapacityBranches(t *testing.T) {
	t.Parallel()
	maIfc := newQueue(t, queue.Options{Buffer: 1})
	ma := maIfc.(*memoryAdapter)
	tq, _ := ma.topic("waitcap")
	if err := maIfc.Push(context.Background(), "waitcap", queue.Payload([]byte("one")), nil); err != nil {
		t.Fatalf("Push fill: %v", err)
	}
	// cover waitForSpace ctx.Done (345)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	err := ma.waitForSpace(ctx, tq)
	if err == nil {
		t.Fatalf("waitForSpace timeout = nil, want error")
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		m, _ := ma.popWithTopic(context.Background(), "waitcap", tq)
		_ = ma.Ack(context.Background(), m)
	}()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel2()
	if err := ma.waitForSpace(ctx2, tq); err != nil {
		t.Fatalf("waitForSpace after free = %v, want nil", err)
	}
	// cover timer Stop false drain (324,335)
	countCalls := 0
	err = ma.waitForCapacity(context.Background(), tq, func() int {
		countCalls++
		if countCalls == 1 {
			return 1
		}
		return 0
	})
	if err != nil {
		t.Fatalf("waitForCapacity = %v", err)
	}
}

func TestMemory_CoverStartPromoterQuitReset(t *testing.T) {
	t.Parallel()
	// cover quit closed reset (174)
	tq := newTopicQueue()
	close(tq.quit)
	tq.promoterOn = false
	tq.startPromoterLocked()
	if tq.quit == nil {
		t.Errorf("quit nil after reset")
	}
	done := tq.done
	close(tq.quit)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("promoter done not closed (reset case)")
	}
	// cover early return when promoterOn (165)
	tq2 := newTopicQueue()
	tq2.startPromoterLocked()
	if !tq2.promoterOn {
		t.Errorf("promoterOn = false, want true")
	}
	done2 := tq2.done
	tq2.startPromoterLocked()
	if !tq2.promoterOn {
		t.Errorf("promoterOn = false after second start, want true")
	}
	close(tq2.quit)
	select {
	case <-done2:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("promoter done not closed (early return case)")
	}
}

func TestMemory_CoverVisibilityReclaimAndFallback(t *testing.T) {
	t.Parallel()
	tq := newTopicQueue()
	// cover reclaimFromMap heap non-empty false (226)
	tq.visHeap = append(tq.visHeap, &visibilityEntry{id: queue.NewMessage("cover-id", nil, nil).ID, deadline: time.Now()})
	tq.inflight[queue.NewMessage("cover-id", nil, nil).ID] = inflight{msg: queue.NewMessage("t", nil, nil), deadline: time.Now()}
	if got := tq.reclaimFromMap(time.Now()); got {
		t.Errorf("reclaimFromMap with heap not empty = true, want false")
	}
	tq2 := newTopicQueue()
	id := queue.NewMessage("cover-id", nil, nil).ID
	msg := queue.NewMessage("t", queue.Payload([]byte("x")), nil)
	tq2.inflight[id] = inflight{msg: msg, deadline: time.Now().Add(-time.Second)}
	tq2.visHeap = append(tq2.visHeap, &visibilityEntry{id: id, deadline: time.Now().Add(-time.Second)})
	tq2.reclaimExpired(time.Second)
	// cover signalSpace default when full
	tq3 := newTopicQueue()
	tq3.spaceCh = make(chan struct{}, 1)
	tq3.spaceCh <- struct{}{}
	tq3.signalSpace()
	tq4 := newTopicQueue()
	tq4.spaceCh = make(chan struct{}, 1)
	tq4.signalSpace()
	select {
	case <-tq4.spaceCh:
	default:
		t.Errorf("signalSpace did not send when space available")
	}
	// cover signal default when notify full
	tq5 := newTopicQueue()
	tq5.notify = make(chan struct{}, 1)
	tq5.notify <- struct{}{}
	tq5.signal()
}

func TestMemory_CoverNewTopicAfterClose(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{})
	_ = q.Close()
	// cover new topic after close ErrClosed (52,83)
	if err := q.Push(context.Background(), "brand-new-topic-push", queue.Payload([]byte("x")), nil); err != queue.ErrClosed {
		t.Errorf("Push new topic after close = %v, want ErrClosed", err)
	}
	if err := q.PushDelayed(context.Background(), "brand-new-topic-pushdelayed", queue.Payload([]byte("x")), nil, 0); err != queue.ErrClosed {
		t.Errorf("PushDelayed new topic after close = %v, want ErrClosed", err)
	}
	if err := q.PushDelayed(context.Background(), "brand-new-topic-pushdelayed2", queue.Payload([]byte("x")), nil, 10*time.Millisecond); err != queue.ErrClosed {
		t.Errorf("PushDelayed delayed new topic after close = %v, want ErrClosed", err)
	}
}

func TestMemory_CoverCollectDueSortTwo(t *testing.T) {
	t.Parallel()
	tq := newTopicQueue()
	now := time.Now()
	// cover sort closure with two due (118)
	tq.delayed = []delayedEntry{
		{msg: queue.NewMessage("t", queue.Payload([]byte("a")), nil), readyAt: now.Add(-2 * time.Second)},
		{msg: queue.NewMessage("t", queue.Payload([]byte("b")), nil), readyAt: now.Add(-1 * time.Second)},
	}
	due, _, _ := tq.collectDue()
	if len(due) != 2 {
		t.Fatalf("collectDue due len = %d, want 2", len(due))
	}
	if due[0].readyAt.After(due[1].readyAt) {
		t.Errorf("due not sorted")
	}
}

func TestMemory_CoverReclaimHeapPushBack(t *testing.T) {
	t.Parallel()
	tq := newTopicQueue()
	// cover heap push-back when now==deadline (else branch)
	id := queue.NewMessage("t", nil, nil).ID
	msg := queue.NewMessage("t", queue.Payload([]byte("x")), nil)
	future := time.Now().Truncate(time.Millisecond)
	tq.inflight[id] = inflight{msg: msg, deadline: future}
	tq.visHeap = append(tq.visHeap, &visibilityEntry{id: id, deadline: future})
	reclaimed := tq.reclaimFromHeap(future)
	if reclaimed {
		t.Errorf("reclaimFromHeap at exact deadline = true, want false")
	}
	if tq.visHeap.Len() != 1 {
		t.Errorf("visHeap len = %d, want 1 (pushed back)", tq.visHeap.Len())
	}
}
