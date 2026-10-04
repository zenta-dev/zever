package otlp

import (
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/observability"
)

func TestInstrumentCache_concurrentSameName_singleInstrument(t *testing.T) {
	t.Parallel()

	mp := testMeterProvider()
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	c := newTestCache()
	const workers = 32

	var wg sync.WaitGroup
	errs := make([]error, workers)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[w] = c.counter(mp, "scope", "shared")
		}()
	}
	wg.Wait()
	for w, err := range errs {
		if err != nil {
			t.Errorf("worker %d counter() = %v, want nil", w, err)
		}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.kinds) != 1 {
		t.Errorf("kinds = %d, want 1 (single instrument for one name)", len(c.kinds))
	}
	if len(c.counters) != 1 {
		t.Errorf("counters = %d, want 1", len(c.counters))
	}
}

func TestAttrsToKeyValue_empty_nil(t *testing.T) {
	t.Parallel()

	if got := attrsToKeyValue(nil, 0); len(got) != 0 {
		t.Errorf("attrsToKeyValue(nil) len = %d, want 0", len(got))
	}
	if got := attrsToKeyValue([]observability.Attr{}, 0); len(got) != 0 {
		t.Errorf("attrsToKeyValue(empty) len = %d, want 0", len(got))
	}
}

func TestNormalizeAttrs_exactLimit_notTruncated(t *testing.T) {
	t.Parallel()

	s := make([]byte, observability.MaxValueLen)
	for i := range s {
		s[i] = 'x'
	}
	got := normalizeAttrs([]observability.Attr{observability.String("k", string(s))}, 0)
	if len(got) != 1 {
		t.Fatalf("normalizeAttrs() len = %d, want 1", len(got))
	}
	sv, ok := got[0].Value.(observability.StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}
	if len(sv.Value) != observability.MaxValueLen {
		t.Errorf("value len = %d, want %d (no truncation at exact limit)", len(sv.Value), observability.MaxValueLen)
	}
}
