package log_test

import (
	"io"
	"testing"

	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	"github.com/zenta-dev/zever/core/notification"
)

// BenchmarkNotify measures the JSON-encode-and-write hot path against
// io.Discard.
func BenchmarkNotify(b *testing.B) {
	n, err := notificationlog.NewWithWriter(notification.Options{}, io.Discard)
	if err != nil {
		b.Fatalf("NewWithWriter() = %v", err)
	}
	defer n.Close()
	in := validNotification()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := n.Notify(b.Context(), in); err != nil {
			b.Fatalf("Notify() = %v, want nil", err)
		}
	}
}

// BenchmarkNotifyParallel measures concurrent Notify calls sharing one writer.
func BenchmarkNotifyParallel(b *testing.B) {
	n, err := notificationlog.NewWithWriter(notification.Options{}, io.Discard)
	if err != nil {
		b.Fatalf("NewWithWriter() = %v", err)
	}
	defer n.Close()
	in := validNotification()
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
