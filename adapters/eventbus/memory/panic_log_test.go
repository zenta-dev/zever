package memory

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/eventbus"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.String()
}

func TestDefaultPanicHandlerLogsStack(t *testing.T) {
	t.Parallel()

	mb := coverMustBus(t, eventbus.Options{BufferSize: 8, MaxHandlers: 4})
	defer mb.Close()

	buf := &syncBuf{}
	mb.logger = slog.New(slog.NewTextHandler(buf, nil))

	ctx := t.Context()

	if _, err := mb.Subscribe(ctx, "boom", func(context.Context, eventbus.Message) {
		panic("kaboom")
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := mb.Publish(ctx, "boom", eventbus.NewPayload([]byte("x")), nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	coverWaitFor(t, "panic log", func() bool { return strings.Contains(buf.String(), "kaboom") })

	out := buf.String()
	for _, want := range []string{"topic=boom", "stack=", "TestDefaultPanicHandlerLogsStack"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q missing %q", out, want)
		}
	}
}

func TestCustomOnPanicSuppressesDefaultLog(t *testing.T) {
	t.Parallel()

	got := make(chan any, 1)
	mb := coverMustBus(t, eventbus.Options{
		BufferSize:  8,
		MaxHandlers: 4,
		OnPanic:     func(_ string, _ eventbus.Message, r any) { got <- r },
	})
	defer mb.Close()

	buf := &syncBuf{}
	mb.logger = slog.New(slog.NewTextHandler(buf, nil))

	ctx := t.Context()

	if _, err := mb.Subscribe(ctx, "boom", func(context.Context, eventbus.Message) { panic("x") }); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := mb.Publish(ctx, "boom", eventbus.NewPayload([]byte("x")), nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	<-got

	if s := buf.String(); s != "" {
		t.Errorf("custom OnPanic set, but default log wrote %q", s)
	}
}
