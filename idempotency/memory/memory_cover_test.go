package memory

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/idempotency"
)

func TestCoverBeginExpiredEntryReclaimed(t *testing.T) {
	t.Parallel()
	st, err := New(idempotency.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s, ok := st.(*store)
	if !ok {
		t.Fatalf("New type = %T, want *store", st)
	}
	s.mu.Lock()
	s.entries[s.prefix+"key-expired"] = &entry{expires: time.Now().Add(-time.Hour)}
	s.mu.Unlock()
	out, err := st.Begin(t.Context(), "key-expired", idempotency.BeginOptions{})
	if err != nil {
		t.Fatalf("Begin after expiry: %v", err)
	}
	if out.Replay {
		t.Fatal("expired record must not replay")
	}
}
