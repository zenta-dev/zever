package sms_test

import (
	"sync"
	"testing"

	sms "github.com/example/zever-sms"
)

// TestStubConcurrentSend pins goroutine-safe Send with no lost messages.
func TestStubConcurrentSend(t *testing.T) {
	t.Parallel()

	svc, err := sms.New(sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const n = 32
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = svc.Send(t.Context(), "+15550001", "hello")
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	senter, ok := svc.(interface{ Sent() []sms.Message })
	if !ok {
		t.Fatal("stub does not expose Sent()")
	}
	if got := len(senter.Sent()); got != n {
		t.Fatalf("sent = %d, want %d", got, n)
	}
}

// TestSentCopyIsolation pins Sent returns a copy, not the live slice.
func TestSentCopyIsolation(t *testing.T) {
	t.Parallel()

	svc, err := sms.New(sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := svc.Send(t.Context(), "+15550001", "one"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	senter, ok := svc.(interface{ Sent() []sms.Message })
	if !ok {
		t.Fatal("stub does not expose Sent()")
	}
	first := senter.Sent()
	first[0].Body = "mutated"
	second := senter.Sent()
	if second[0].Body != "one" {
		t.Fatalf("Sent copy mutated: %q", second[0].Body)
	}
}

// TestNewInstancesAreIndependent pins the code-wiring contract: every New
// call returns an isolated backend with no process-global state, so hosts
// switch backends by calling a different constructor and rebuilding.
func TestNewInstancesAreIndependent(t *testing.T) {
	t.Parallel()

	first, err := sms.New(sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	second, err := sms.New(sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := first.Send(t.Context(), "+15550001", "one"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	firstSent, ok := first.(interface{ Sent() []sms.Message })
	if !ok {
		t.Fatal("stub does not expose Sent()")
	}
	secondSent, ok := second.(interface{ Sent() []sms.Message })
	if !ok {
		t.Fatal("stub does not expose Sent()")
	}

	if got := len(firstSent.Sent()); got != 1 {
		t.Fatalf("first sent = %d, want 1", got)
	}
	if got := len(secondSent.Sent()); got != 0 {
		t.Fatalf("second sent = %d, want 0 (independent instance)", got)
	}
}
