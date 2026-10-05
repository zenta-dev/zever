package queue

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/webhook"
)

func TestEdgeRegisterEmptySecretAccepted(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	if err := a.Register(t.Context(), "e", "http://127.0.0.1:1/a", ""); err != nil {
		t.Fatalf("Register empty secret err = %v, want nil", err)
	}

	a.mu.RLock()
	reg := a.regs["e"]["http://127.0.0.1:1/a"]
	a.mu.RUnlock()

	if reg.secret != "" {
		t.Fatalf("secret = %q, want empty stored", reg.secret)
	}

	// An empty secret can never verify: consumption dead-letters the message.
	payload := []byte(`{}`)
	msg := testMessage("webhook:e", payload, map[string]string{
		"X-Webhook-Target":    "http://127.0.0.1:1/a",
		"X-Webhook-Signature": signPayloadAt("", payload, time.Now().Unix()),
	}, 1)

	a.processMessage("e", msg)

	sq, ok := a.queue.(*stubQueue)
	if !ok {
		t.Fatalf("queue type = %T, want *stubQueue", a.queue)
	}

	assertDeadLetter(t, sq, ErrSignatureMismatch.Error())
}

func TestEdgeRegisterMalformedTargetSyntax(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	for _, target := range []string{"://bad", "http://", "not a url"} {
		err := a.Register(t.Context(), "e", target, "s")
		if err == nil {
			t.Fatalf("Register target %q expected error, got nil", target)
		}

		if !strings.Contains(err.Error(), "queue: reject target") {
			t.Fatalf("Register target %q err = %v, want reject target prefix", target, err)
		}
	}
}

func TestEdgeUnregisterEmptyEvent(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	if err := a.Unregister(t.Context(), "", "http://127.0.0.1:1/a"); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Unregister empty event err = %v, want ErrNotFound", err)
	}
}

func TestEdgeDeliverEmptyPayload(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	sq := newStubQueue()
	a := newTestAdapter(sq)
	defer func() { _ = a.Close() }()

	a.regs["e"] = regsWith(nil, "http://127.0.0.1:1/a", "s")

	if err := a.Deliver(t.Context(), "e", nil); err != nil {
		t.Fatalf("Deliver nil payload err = %v", err)
	}

	if sq.pushCount() != 1 {
		t.Fatalf("pushes = %d, want 1", sq.pushCount())
	}

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.pushes[0].payload) != 0 {
		t.Fatalf("payload = %q, want empty", sq.pushes[0].payload)
	}

	if !verifyHMAC("s", nil, sq.pushes[0].headers["X-Webhook-Signature"]) {
		t.Fatal("signature does not verify against empty payload")
	}
}

func TestEdgeDeliverSingleTargetPushError(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	sq := newStubQueue()
	sq.pushErr = errTestTransport
	a := newTestAdapter(sq)
	defer func() { _ = a.Close() }()

	a.regs["e"] = regsWith(nil, "http://127.0.0.1:1/a", "s")

	err := a.Deliver(t.Context(), "e", []byte(`{}`))
	if !errors.Is(err, errTestTransport) {
		t.Fatalf("Deliver err = %v, want errTestTransport", err)
	}

	if !strings.Contains(err.Error(), "queue: push to queue") {
		t.Fatalf("Deliver err = %v, want push prefix", err)
	}
}

func TestEdgeProcessMessageEmptyPayload(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	var mu sync.Mutex

	count := 0
	srv := okServer(t, "s", &mu, &count)
	defer srv.Close()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	defer func() { _ = a.Close() }()

	target := srv.URL + "/hook"
	a.regs["e"] = regsWith(nil, target, "s")

	msg := testMessage("webhook:e", nil, map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": signPayload("s", nil),
	}, 1)

	a.processMessage("e", msg)

	if sq.ackCount() != 1 {
		t.Fatalf("acks = %d, want 1", sq.ackCount())
	}

	mu.Lock()
	defer mu.Unlock()

	if count != 1 {
		t.Fatalf("server hits = %d, want 1", count)
	}
}

func TestEdgeProcessMessageAttemptBoundaryRequeue(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	sq := newStubQueue()
	a := newTestAdapter(sq)
	defer func() { _ = a.Close() }()

	a.regs["e"] = regsWith(nil, "http://127.0.0.1:1/a", "s")

	msg := testMessage("webhook:e", []byte(`{}`), map[string]string{
		"X-Webhook-Target":    "http://127.0.0.1:1/a",
		"X-Webhook-Signature": signPayload("s", []byte(`{}`)),
		"X-Webhook-Attempt":   "2",
	}, 1)

	a.processMessage("e", msg)

	if sq.delayedCount() != 1 {
		t.Fatalf("delayed = %d, want 1 requeue at attempt cap-1", sq.delayedCount())
	}

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if sq.delayed[0].push.headers["X-Webhook-Attempt"] != "3" {
		t.Fatalf("attempt = %q, want 3", sq.delayed[0].push.headers["X-Webhook-Attempt"])
	}
}

func TestEdgeParseSignatureHeaderMalformed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		header  string
		wantOK  bool
		wantTS  int64
		wantMac string
	}{
		{"empty", "", false, 0, ""},
		{"garbage", "garbage", false, 0, ""},
		{"missing v1", "t=1", false, 0, ""},
		{"missing t defaults zero", "v1=aa", true, 0, "aa"},
		{"bad ts", "t=abc,v1=aa", false, 0, ""},
		{"empty mac", "t=1,v1=", false, 0, ""},
		{"extra fields ignored", "t=1,v1=aa,extra=1", true, 1, "aa"},
		{"wrong-case t ignored", "T=1,v1=aa", true, 0, "aa"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ts, mac, ok := parseSignatureHeader(c.header)
			if ok != c.wantOK || ts != c.wantTS || mac != c.wantMac {
				t.Fatalf("parseSignatureHeader(%q) = %d,%q,%v want %d,%q,%v",
					c.header, ts, mac, ok, c.wantTS, c.wantMac, c.wantOK)
			}
		})
	}
}

func TestEdgeBuildSignatureHeaderDeterministic(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	payload := []byte(`{"a":1}`)

	a, b := buildSignatureHeader("s", payload, 1_700_000_000), buildSignatureHeader("s", payload, 1_700_000_000)
	if a != b {
		t.Fatalf("buildSignatureHeader not deterministic: %q vs %q", a, b)
	}

	if !strings.HasPrefix(a, "t=1700000000,v1=") {
		t.Fatalf("buildSignatureHeader = %q, want envelope prefix", a)
	}

	if other := buildSignatureHeader("s", payload, 1_700_000_001); a == other {
		t.Fatal("buildSignatureHeader must differ across timestamps")
	}

	if other := buildSignatureHeader("other", payload, 1_700_000_000); a == other {
		t.Fatal("buildSignatureHeader must differ across secrets")
	}

	if empty := buildSignatureHeader("s", nil, 1_700_000_000); empty == "" {
		t.Fatal("buildSignatureHeader empty payload = empty string")
	}
}

func TestEdgeWorkerEventNamesSnapshot(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	w := newWorker()

	for _, e := range []string{"e1", "e2", "e3"} {
		w.addEvent(e)
	}

	got := w.eventNames()
	if len(got) != 3 {
		t.Fatalf("eventNames = %v, want 3 entries", got)
	}

	seen := make(map[string]bool, len(got))
	for _, e := range got {
		seen[e] = true
	}

	for _, e := range []string{"e1", "e2", "e3"} {
		if !seen[e] {
			t.Fatalf("eventNames missing %q", e)
		}
	}
}

func TestEdgeCloseNilWorkers(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	sq := newStubQueue()
	a := &adapter{
		regs:        make(map[string]map[string]registration),
		queue:       sq,
		timeout:     time.Second,
		consecutive: make(map[string]int),
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	if sq.closes != 1 {
		t.Fatalf("queue closes = %d, want 1", sq.closes)
	}
}

func TestEdgeConsumeOnePopError(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	sq := newStubQueue()
	sq.popErr = errTestTransport
	a := newTestAdapter(sq)
	defer func() { _ = a.Close() }()

	if a.consumeOne("e") {
		t.Fatal("consumeOne reported work for a failed pop")
	}

	a.mu.Lock()
	n := a.consecutive["e"]
	a.mu.Unlock()

	if n != 1 {
		t.Fatalf("consecutive = %d, want 1", n)
	}
}

func TestEdgeEffectiveAttemptNegativeHeader(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	msg := testMessage("t", []byte(`{}`), map[string]string{"X-Webhook-Attempt": "-5"}, 2)
	if got := a.effectiveAttempt(msg); got != 2 {
		t.Fatalf("effectiveAttempt = %d, want 2 (negative header ignored)", got)
	}
}

func TestEdgeConcurrentRegisterUnregister(t *testing.T) {
	// sequential: background consumer workers must not overlap the parallel phase

	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	var wg sync.WaitGroup

	for i := range 24 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			event := fmt.Sprintf("edge%c", 'a'+i%6)
			target := "http://127.0.0.1:1/" + event
			_ = a.Register(t.Context(), event, target, "s")
			_ = a.Unregister(t.Context(), event, target)
		}()
	}

	wg.Wait()
}
