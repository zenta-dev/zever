package sms_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	sms "github.com/example/zever-sms"
)

// TestStubConcurrentSend pins goroutine-safe Send with no lost messages.
func TestStubConcurrentSend(t *testing.T) {
	t.Parallel()

	if err := ensureRegistered(); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}
	svc, err := sms.Open(sms.Stub, sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("Open: %v", err)
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

	if err := ensureRegistered(); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}
	svc, err := sms.Open(sms.Stub, sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("Open: %v", err)
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

// TestRegisterDuplicate pins duplicate adapter registration error.
func TestRegisterDuplicate(t *testing.T) {
	t.Parallel()

	if err := ensureRegistered(); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}
	err := sms.Register(sms.Stub, func(_ sms.Options) (sms.SMS, error) {
		return nil, errors.New("duplicate factory")
	})
	if !errors.Is(err, sms.ErrDuplicate) {
		t.Fatalf("duplicate Register = %v, want ErrDuplicate", err)
	}
	if !strings.Contains(err.Error(), "stub") {
		t.Fatalf("duplicate error = %q, want adapter name", err.Error())
	}
}
