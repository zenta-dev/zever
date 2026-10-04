package notification

import "testing"

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Register(freshAdapter(), func(Options) (Notifier, error) { return &stubNotifier{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Notifier, error) { return &stubNotifier{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open err = %v", err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Notifier, error) { return &stubNotifier{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Fatalf("Open err = %v", err)
			}
		}
	})
}

func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseAdapter("fcm")
	}
}

func BenchmarkNotificationValidate(b *testing.B) {
	n := Notification{
		Target:   "+14155552671",
		Channel:  ChannelSMS,
		Body:     "hello",
		Priority: PriorityHigh,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := n.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

func BenchmarkNotificationClone(b *testing.B) {
	n := Notification{
		Target:  "token",
		Channel: ChannelPush,
		Body:    "hello",
		Data:    map[string]string{"a": "1", "b": "2"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = n.Clone()
	}
}

func BenchmarkNewNotification(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewNotification("token", ChannelPush, "body")
	}
}

func BenchmarkNotifierNotify(b *testing.B) {
	n := &stubNotifier{}
	msg := &Notification{Target: "token", Channel: ChannelPush, Body: "hello"}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := n.Notify(ctx, msg); err != nil {
			b.Fatalf("Notify err = %v", err)
		}
	}
}
