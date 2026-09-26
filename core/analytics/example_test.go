package analytics_test

import (
	"bytes"
	"context"
	"fmt"

	"github.com/zenta-dev/zever/analytics"
	analyzelog "github.com/zenta-dev/zever/analytics/log"
)

// ExampleOpen opens the log backend against a buffer and tracks one event.
func ExampleOpen() {
	var buf bytes.Buffer

	backend, err := analyzelog.NewWithWriter(analytics.Options{}, &buf)
	if err != nil {
		fmt.Println("open error")
		return
	}
	defer backend.Close()

	ctx := analytics.WithUserID(context.Background(), "user-1")
	if err := backend.Track(ctx, "signup", map[string]any{"plan": "free"}); err != nil {
		fmt.Println("track error")
		return
	}

	fmt.Println("tracked")
	// Output: tracked
}
