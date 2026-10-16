package memory

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.b.String()
}

// TestPrunerSurvivesPanic corrupts the expiry heap so the first sweep panics,
// then checks the panic is logged and a later tick still prunes.
func TestPrunerSurvivesPanic(t *testing.T) {
	// Not parallel: swaps the package-level newTicker seam.
	fired := make(chan time.Time)
	old := newTicker
	newTicker = func(time.Duration) *time.Ticker { return &time.Ticker{C: fired} }
	defer func() { newTicker = old }()

	si, err := New(Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = si.Close() })

	s, ok := si.(*store)
	if !ok {
		t.Fatalf("New() returned %T, want *store", si)
	}

	buf := &lockedBuf{}

	s.mu.Lock()
	s.logger = slog.New(slog.NewTextHandler(buf, nil))
	s.byExpiry = expiryHeap{nil} // nil root: the sweep dereferences it and panics
	s.mu.Unlock()

	// Unbuffered: a send completes once the pruner is back at its select, so
	// two sends in a row guarantee the previous sweep finished.
	fired <- time.Now()
	fired <- time.Now() // returns only after the panicking sweep ended

	s.mu.Lock()
	s.revoked["old"] = time.Now().Add(-time.Minute)
	s.byExpiry = expiryHeap{{jti: "old", expiry: time.Now().Add(-time.Minute)}}
	s.mu.Unlock()

	fired <- time.Now()
	fired <- time.Now() // returns only after the second sweep completed

	s.mu.Lock()
	_, stillThere := s.revoked["old"]
	s.mu.Unlock()

	if stillThere {
		t.Error("pruner did not keep running after a panic")
	}

	if out := buf.String(); !strings.Contains(out, "panic") || !strings.Contains(out, "stack=") {
		t.Errorf("panic not logged with stack: %q", out)
	}
}
