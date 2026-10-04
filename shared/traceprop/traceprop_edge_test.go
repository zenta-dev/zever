package traceprop

import "testing"

// TestInject_overwritesExistingTraceparent verifies a stale incoming
// traceparent is replaced with the caller's span context and the input map is
// left untouched.
func TestInject_overwritesExistingTraceparent(t *testing.T) {
	t.Parallel()

	const stale = "00-00000000000000000000000000000000-0000000000000000-00"

	ctx := ctxWithSpanContext(t.Context(), mustTraceID(t), mustSpanID(t), true)
	in := map[string]string{TraceParentHeader: stale}

	out := Inject(ctx, in)

	const want = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if out[TraceParentHeader] != want {
		t.Errorf("traceparent = %q, want %q", out[TraceParentHeader], want)
	}

	if in[TraceParentHeader] != stale {
		t.Errorf("input mutated: %q", in[TraceParentHeader])
	}
}

// TestExtract_nilHeadersReturnsContext verifies extraction of a nil map leaves
// the context unchanged.
func TestExtract_nilHeadersReturnsContext(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	if got := Extract(ctx, nil); got != ctx {
		t.Error("Extract(nil) returned a different context")
	}
}
