package observability_test

import (
	"context"

	"github.com/zenta-dev/zever/adapters/observability/noop"
	"github.com/zenta-dev/zever/core/observability"
)

// ExampleOpen opens a noop provider and records a span.
func ExampleOpen() {
	_ = observability.Register(observability.Noop, func(_ observability.Options) (observability.Provider, error) {
		return noop.New(), nil
	})

	provider, err := observability.Open(observability.Noop, observability.Options{ServiceName: "example"})
	if err != nil {
		return
	}

	ctx := context.Background()
	_, span := provider.Tracer("example").Start(ctx, "op")
	span.End()
	_ = provider.Shutdown(ctx)
}
