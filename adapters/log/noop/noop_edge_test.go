package noop

import (
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/log"
)

func TestEdge_EventChainWithNilError(t *testing.T) {
	t.Parallel()

	ev := New().Info().Err(nil).AnErr("cause", nil).Any("a", nil)
	if ev == nil {
		t.Fatal("event chain = nil, want non-nil")
	}

	ev.Msg("ok")
}

func TestEdge_ContextChainWithNilError(t *testing.T) {
	t.Parallel()

	c := New().With().Err(nil).AnErr("cause", nil).Any("a", nil)
	if c == nil {
		t.Fatal("context chain = nil, want non-nil")
	}

	if c.Logger() == nil {
		t.Fatal("Logger() = nil, want logger")
	}
}

func TestEdge_AllLevelsDiscardWithoutPanic(t *testing.T) {
	t.Parallel()

	l := New()

	for _, ev := range []log.Event{l.Debug(), l.Info(), l.Warn(), l.Error(), l.Fatal()} {
		ev.Str("k", "v").Dur("d", time.Second).Send()
	}
}

func TestEdge_ConcurrentEmit(t *testing.T) {
	t.Parallel()

	l := New()

	const goroutines = 32

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := range goroutines {
		go func(i int) {
			defer wg.Done()

			l.Info().Int("i", i).Msg("concurrent")
			_ = l.WithContext(t.Context())
		}(i)
	}

	wg.Wait()
}
