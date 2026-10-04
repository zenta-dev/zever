package queue

import (
	"testing"
	"time"
)

// benchQueueAdapter registers a stub queue once and returns its adapter so
// Open can be measured without paying the one-shot registration cost.
func benchQueueAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshQueueAdapter()
	if err := Register(a, func(Options) (Queue, error) { return stubQueue{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkOpen(b *testing.B) {
	a := benchQueueAdapter(b)
	opts := Options{VisibilityTimeout: time.Minute}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := Open(a, opts); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := benchQueueAdapter(b)
	opts := Options{VisibilityTimeout: time.Minute}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, opts); err != nil {
				b.Errorf("Open(%v) error = %v", a, err)
				return
			}
		}
	})
}

func BenchmarkNewMessage(b *testing.B) {
	payload := NewPayload([]byte("hello world"))
	headers := NewHeaders(map[string]string{"content-type": "text/plain"})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		msg := NewMessage("jobs", payload, headers)
		if msg.Attempt != 1 {
			b.Fatalf("NewMessage Attempt = %d, want 1", msg.Attempt)
		}
	}
}

func BenchmarkMessageClone(b *testing.B) {
	msg := NewMessage("jobs", NewPayload([]byte("hello world")), NewHeaders(map[string]string{"k": "v"}))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if cloned := msg.Clone(); len(cloned.Payload) != len(msg.Payload) {
			b.Fatal("Message.Clone lost payload")
		}
	}
}

func BenchmarkParseMessageID(b *testing.B) {
	id := newMessageID().String()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := ParseMessageID(id); err != nil {
			b.Fatalf("ParseMessageID(%q) error = %v", id, err)
		}
	}
}

func BenchmarkHeadersClone(b *testing.B) {
	headers := NewHeaders(map[string]string{"a": "1", "b": "2", "c": "3"})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if cloned := headers.Clone(); len(cloned) != len(headers) {
			b.Fatal("Headers.Clone lost entries")
		}
	}
}

func BenchmarkPayloadClone(b *testing.B) {
	payload := NewPayload([]byte("hello world"))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if cloned := payload.Clone(); len(cloned) != len(payload) {
			b.Fatal("Payload.Clone lost bytes")
		}
	}
}
