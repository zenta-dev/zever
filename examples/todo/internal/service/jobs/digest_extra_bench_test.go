package jobs_test

import (
	"io"
	"testing"

	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/examples/todo/internal/service/jobs"
)

// BenchmarkRunDigest measures the overdue digest scan plus count.
func BenchmarkRunDigest(b *testing.B) {
	conn, ctx := benchDB(b)
	logger := logslog.NewWithWriter(log.Options{}, io.Discard)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := jobs.RunDigest(ctx, conn, logger); err != nil {
			b.Fatalf("RunDigest: %v", err)
		}
	}
}
