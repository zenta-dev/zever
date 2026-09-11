package log

import (
	"context"
	"testing"
)

func TestContextWithLogger_roundTrip_returnsLogger(t *testing.T) {
	t.Parallel()

	want := stubLogger{}
	ctx := ContextWithLogger(context.Background(), want)

	got := FromContext(ctx, stubLogger{level: LevelFatal})
	if got != want {
		t.Errorf("FromContext() = %+v, want %+v", got, want)
	}
}

func TestFromContext_missing_returnsFallback(t *testing.T) {
	t.Parallel()

	fallback := stubLogger{level: LevelError}

	if got := FromContext(context.Background(), fallback); got != fallback {
		t.Errorf("FromContext() = %+v, want fallback %+v", got, fallback)
	}
}

func TestFromContext_wrongType_returnsFallback(t *testing.T) {
	t.Parallel()

	type otherKey struct{}
	fallback := stubLogger{}

	ctx := context.WithValue(context.Background(), otherKey{}, "not a logger")
	if got := FromContext(ctx, fallback); got != fallback {
		t.Errorf("FromContext() = %+v, want fallback %+v", got, fallback)
	}
}

func TestFromContext_nilFallback_returnsNil(t *testing.T) {
	t.Parallel()

	if got := FromContext(context.Background(), nil); got != nil {
		t.Errorf("FromContext() = %v, want nil", got)
	}
}
