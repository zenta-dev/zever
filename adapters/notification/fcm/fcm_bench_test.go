package fcm

import (
	"context"
	"sync/atomic"
	"testing"

	"firebase.google.com/go/v4/messaging"

	"github.com/zenta-dev/zever/core/notification"
)

func benchNotification() *notification.Notification {
	return &notification.Notification{
		Target:   "device-token-bench",
		Channel:  notification.ChannelPush,
		Title:    "bench",
		Body:     "body",
		Priority: notification.PriorityHigh,
		Data:     map[string]string{"k": "v"},
	}
}

// BenchmarkNotify measures the validation and message-mapping hot path with a
// no-op in-process send.
func BenchmarkNotify(b *testing.B) {
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		return "id", nil
	}}
	in := benchNotification()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := n.Notify(b.Context(), in); err != nil {
			b.Fatalf("Notify() = %v, want nil", err)
		}
	}
}

// BenchmarkNotifyParallel measures concurrent Notify calls against a shared
// notifier with an atomic send counter.
func BenchmarkNotifyParallel(b *testing.B) {
	var calls atomic.Int64
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		calls.Add(1)
		return "id", nil
	}}
	in := benchNotification()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := n.Notify(b.Context(), in); err != nil {
				b.Errorf("Notify() = %v, want nil", err)
				return
			}
		}
	})
}

// BenchmarkNotifyHighPriority measures the high-priority mapping path.
func BenchmarkNotifyHighPriority(b *testing.B) {
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		return "id", nil
	}}
	in := validPush()
	in.Priority = notification.PriorityHigh
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := n.Notify(b.Context(), in); err != nil {
			b.Fatalf("Notify() = %v, want nil", err)
		}
	}
}

// BenchmarkNotifyWithData measures the path that copies the data map.
func BenchmarkNotifyWithData(b *testing.B) {
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		return "id", nil
	}}
	in := validPush()
	in.Data = map[string]string{"order": "42", "sku": "abc"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := n.Notify(b.Context(), in); err != nil {
			b.Fatalf("Notify() = %v, want nil", err)
		}
	}
}

// BenchmarkClose measures the idempotent close.
func BenchmarkClose(b *testing.B) {
	n := &notifier{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := n.Close(); err != nil {
			b.Fatalf("Close() = %v, want nil", err)
		}
	}
}

// BenchmarkValidateServiceAccountPath measures the path guard.
func BenchmarkValidateServiceAccountPath(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := validateServiceAccountPath("service-account.json"); err != nil {
			b.Fatalf("validateServiceAccountPath() = %v, want nil", err)
		}
	}
}
