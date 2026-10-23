package notification

import "testing"

func BenchmarkNotificationValidate(b *testing.B) {
	n := Notification{
		Target:   "+14155552671",
		Channel:  ChannelSMS,
		Body:     "hello",
		Priority: PriorityHigh,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
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
	for b.Loop() {
		_ = n.Clone()
	}
}

func BenchmarkNewNotification(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = NewNotification("token", ChannelPush, "body")
	}
}

func BenchmarkNotifierNotify(b *testing.B) {
	n := &stubNotifier{}
	msg := &Notification{Target: "token", Channel: ChannelPush, Body: "hello"}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := n.Notify(ctx, msg); err != nil {
			b.Fatalf("Notify err = %v", err)
		}
	}
}
