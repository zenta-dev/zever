package eventbus

import (
	"context"
	"testing"
	"time"
)

// BenchmarkRegister measures registry insertion for a fresh adapter key.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if err := Register(freshAdapter(), func(Options) (EventBus, error) { return stubBus{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

// BenchmarkOpen measures validated lookup plus the pull-wrapper upgrade.
func BenchmarkOpen(b *testing.B) {
	adapter := freshAdapter()
	if err := Register(adapter, func(Options) (EventBus, error) { return stubBus{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		bus, err := Open(adapter, Options{})
		if err != nil {
			b.Fatalf("Open err = %v", err)
		}

		_ = bus.Close()
	}
}

// BenchmarkPublish measures the push publish path through a wrapped pusher.
func BenchmarkPublish(b *testing.B) {
	bus := Wrap(newFakeBus())
	ctx := b.Context()

	unsub, err := bus.Subscribe(ctx, "orders", func(context.Context, Message) {})
	if err != nil {
		b.Fatalf("Subscribe err = %v", err)
	}

	defer unsub()

	payload := NewPayload([]byte("payload"))

	b.ReportAllocs()

	for b.Loop() {
		if err := bus.Publish(ctx, "orders", payload, nil); err != nil {
			b.Fatalf("Publish err = %v", err)
		}
	}
}

// BenchmarkNewMessage measures message construction with a fresh UUIDv7 ID.
func BenchmarkNewMessage(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewMessage("orders", NewPayload([]byte("hi")), NewHeaders(map[string]string{"k": "v"}))
	}
}

// BenchmarkMessageClone measures deep-copying a message payload and headers.
func BenchmarkMessageClone(b *testing.B) {
	msg := NewMessage("orders", NewPayload([]byte("hi")), NewHeaders(map[string]string{"k": "v"}))

	b.ReportAllocs()

	for b.Loop() {
		_ = msg.Clone()
	}
}

// BenchmarkParseMessageID measures message ID parsing.
func BenchmarkParseMessageID(b *testing.B) {
	id := newMessageID().String()

	b.ReportAllocs()

	for b.Loop() {
		if _, err := ParseMessageID(id); err != nil {
			b.Fatalf("ParseMessageID err = %v", err)
		}
	}
}

// BenchmarkValidateTopic measures topic shape validation.
func BenchmarkValidateTopic(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if err := validateTopic("orders"); err != nil {
			b.Fatalf("validateTopic err = %v", err)
		}
	}
}

// BenchmarkOptionsValidate measures option validation.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{
		BufferSize:     1024,
		MaxHandlers:    128,
		HandlerTimeout: 30 * time.Second,
		CloseTimeout:   5 * time.Second,
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}
