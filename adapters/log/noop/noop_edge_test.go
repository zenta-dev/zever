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

func TestEdge_EnabledUnknownLevel(t *testing.T) {
	t.Parallel()

	l := New()

	if l.Enabled(log.Level(99)) {
		t.Error("Enabled(unknown) = true, want false")
	}

	if l.Enabled(log.Level(0)) {
		t.Error("Enabled(zero) = true, want false")
	}
}

func TestEdge_WithContextNilContext(t *testing.T) {
	t.Parallel()

	l := New()

	if got := l.WithContext(nil); got != l { //nolint:staticcheck // deliberately exercises nil-context handling
		t.Errorf("WithContext(nil) = %v, want same logger", got)
	}
}

func TestEdge_ChainsReturnSameInstance(t *testing.T) {
	t.Parallel()

	ts := time.Unix(0, 0)

	ev := New().Info()

	eventChains := map[string]func(log.Event) log.Event{ //nolint:dupl // mirrors contextChains with the Event method set
		"Str":     func(e log.Event) log.Event { return e.Str("s", "v") },
		"Int":     func(e log.Event) log.Event { return e.Int("i", 1) },
		"Int64":   func(e log.Event) log.Event { return e.Int64("i64", 1) },
		"Float64": func(e log.Event) log.Event { return e.Float64("f", 1.5) },
		"Bool":    func(e log.Event) log.Event { return e.Bool("b", true) },
		"Dur":     func(e log.Event) log.Event { return e.Dur("d", time.Second) },
		"Time":    func(e log.Event) log.Event { return e.Time("t", ts) },
		"Err":     func(e log.Event) log.Event { return e.Err(nil) },
		"AnErr":   func(e log.Event) log.Event { return e.AnErr("e", nil) },
		"Any":     func(e log.Event) log.Event { return e.Any("a", nil) },
	}

	for name, chain := range eventChains {
		if got := chain(ev); got != ev {
			t.Errorf("event %s() = different event, want same instance", name)
		}
	}

	c := New().With()

	contextChains := map[string]func(log.Context) log.Context{ //nolint:dupl // mirrors eventChains with the Context method set
		"Str":     func(x log.Context) log.Context { return x.Str("s", "v") },
		"Int":     func(x log.Context) log.Context { return x.Int("i", 1) },
		"Int64":   func(x log.Context) log.Context { return x.Int64("i64", 1) },
		"Float64": func(x log.Context) log.Context { return x.Float64("f", 1.5) },
		"Bool":    func(x log.Context) log.Context { return x.Bool("b", true) },
		"Dur":     func(x log.Context) log.Context { return x.Dur("d", time.Second) },
		"Time":    func(x log.Context) log.Context { return x.Time("t", ts) },
		"Err":     func(x log.Context) log.Context { return x.Err(nil) },
		"AnErr":   func(x log.Context) log.Context { return x.AnErr("e", nil) },
		"Any":     func(x log.Context) log.Context { return x.Any("a", nil) },
	}

	for name, chain := range contextChains {
		if got := chain(c); got != c {
			t.Errorf("context %s() = different context, want same instance", name)
		}
	}
}

func TestEdge_MsgfNoArgs(t *testing.T) {
	t.Parallel()

	l := New()

	l.Info().Msgf("plain")
	l.Info().Msgf("")
	l.Info().Msgf("%d", 1)
}

func TestEdge_SendIdempotent(t *testing.T) {
	t.Parallel()

	ev := New().Info().Str("k", "v")

	ev.Send()
	ev.Send()
}

func TestEdge_ConcurrentWithContext(t *testing.T) {
	t.Parallel()

	l := New()

	const goroutines = 32

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			l.With().Str("k", "v").Logger().Info().Msg("child")
		}()
	}

	wg.Wait()
}
