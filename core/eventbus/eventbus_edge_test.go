package eventbus

import (
	"strings"
	"sync"
	"testing"
)

func TestEventbusEdge_ValidateTopicBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		topic   string
		wantErr bool
	}{
		{name: "one valid", topic: "a"},
		{name: "max valid", topic: strings.Repeat("a", MaxTopicLen)},
		{name: "empty invalid", topic: "", wantErr: true},
		{name: "too long invalid", topic: strings.Repeat("a", MaxTopicLen+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := validateTopic(tt.topic) != nil; got != tt.wantErr {
				t.Fatalf("validateTopic() error present = %v, want %v", got, tt.wantErr)
			}
		})
	}
}

func TestEventbusEdge_OpenConcurrent(t *testing.T) {
	t.Parallel()

	adapter := freshAdapter()
	if err := Register(adapter, func(Options) (EventBus, error) { return stubBus{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			bus, err := Open(adapter, Options{})
			if err != nil {
				t.Errorf("Open err = %v", err)

				return
			}

			_ = bus.Close()
		}()
	}

	wg.Wait()
}

func TestEventbusEdge_RegisterConcurrentUnique(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	errs := make([]error, 20)

	for i := range errs {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			errs[i] = Register(freshAdapter(), func(Options) (EventBus, error) { return stubBus{}, nil })
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Register worker %d err = %v", i, err)
		}
	}
}

func TestEventbusEdge_CloneNilSlices(t *testing.T) {
	t.Parallel()

	msg := Message{}

	cloned := msg.Clone()
	if cloned.Payload != nil || cloned.Headers != nil {
		t.Fatalf("Clone() = %+v, want nil payload and headers", cloned)
	}

	var headers Headers

	if got := headers.Clone(); got != nil {
		t.Fatalf("Headers.Clone(nil) = %v, want nil", got)
	}

	var payload Payload

	if got := payload.Clone(); got != nil {
		t.Fatalf("Payload.Clone(nil) = %v, want nil", got)
	}
}

func TestEventbusEdge_DefaultChanBuffer(t *testing.T) {
	t.Parallel()

	if DefaultChanBuffer != 256 {
		t.Fatalf("DefaultChanBuffer = %d, want 256", DefaultChanBuffer)
	}
}
