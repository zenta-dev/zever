package eventbus

import "testing"

func BenchmarkOpen(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (EventBus, error) { return stubBus{}, nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (EventBus, error) { return stubBus{}, nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkNewMessage(b *testing.B) {
	payload := NewPayload([]byte("hello"))
	headers := NewHeaders(map[string]string{"trace": "abc"})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = NewMessage("orders.created", payload, headers)
	}
}

func BenchmarkMessageClone(b *testing.B) {
	m := NewMessage("orders.created", NewPayload([]byte("hello")), NewHeaders(map[string]string{"trace": "abc"}))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = m.Clone()
	}
}

func BenchmarkParseMessageID(b *testing.B) {
	id := NewMessage("t", nil, nil).ID.String()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := ParseMessageID(id); err != nil {
			b.Fatal(err)
		}
	}
}
