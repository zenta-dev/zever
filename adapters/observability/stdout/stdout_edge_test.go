package stdout_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/adapters/observability/stdout"
	"github.com/zenta-dev/zever/core/observability"
)

func TestSpan_unknownAttrValueType_encodesNull(t *testing.T) {
	t.Parallel()

	p, buf := openBuffered(t, nil)
	_, span := p.Tracer("s").Start(t.Context(), "op")
	span.SetAttributes(observability.Attr{Key: "weird", Value: nil})
	span.End()

	v := decodeLine(t, buf)
	attrs, ok := v["attrs"].(map[string]any)
	if !ok {
		t.Fatalf("attrs = %v, want map", v["attrs"])
	}
	got, present := attrs["weird"]
	if !present {
		t.Fatal("attrs missing weird key")
	}
	if got != nil {
		t.Errorf("weird = %v, want nil for unknown value type", got)
	}
}

func TestSpan_setAttributesEmpty_omitsAttrs(t *testing.T) {
	t.Parallel()

	p, buf := openBuffered(t, nil)
	_, span := p.Tracer("s").Start(t.Context(), "op")
	span.SetAttributes()
	span.End()

	v := decodeLine(t, buf)
	if _, ok := v["attrs"]; ok {
		t.Errorf("attrs = %v, want omitted when none set", v["attrs"])
	}
}

func TestSpan_attrExactlyLimit_notTruncated(t *testing.T) {
	t.Parallel()

	p, buf := openBuffered(t, nil)
	want := strings.Repeat("x", observability.MaxValueLen)
	_, span := p.Tracer("s").Start(t.Context(), "op")
	span.SetAttributes(observability.String("k", want))
	span.End()

	v := decodeLine(t, buf)
	attrs, ok := v["attrs"].(map[string]any)
	if !ok {
		t.Fatalf("attrs = %T, want map[string]any", v["attrs"])
	}
	got, ok := attrs["k"].(string)
	if !ok {
		t.Fatalf("attr = %T, want string", attrs["k"])
	}
	if got != want {
		t.Errorf("attr len = %d, want %d (no truncation at exact limit)", len(got), len(want))
	}
}

func TestSpan_attrOverLimit_truncated(t *testing.T) {
	t.Parallel()

	p, buf := openBuffered(t, nil)
	want := strings.Repeat("x", observability.MaxValueLen)
	_, span := p.Tracer("s").Start(t.Context(), "op")
	span.SetAttributes(observability.String("k", want+"tail"))
	span.End()

	v := decodeLine(t, buf)
	attrs, ok := v["attrs"].(map[string]any)
	if !ok {
		t.Fatalf("attrs = %T, want map[string]any", v["attrs"])
	}
	if got := attrs["k"]; got != want {
		t.Errorf("attr = %q, want truncated to %d chars", got, len(want))
	}
}

func TestMetrics_nilContext_verboseEmits(t *testing.T) {
	t.Parallel()

	p, buf := openBuffered(t, func(o *observability.Options) { o.Verbose = true })
	if err := p.Meter("s").Counter(nil, "c", 1, observability.String("k", "v")); err != nil { //nolint:staticcheck // deliberately exercises nil-context handling
		t.Fatalf("Counter(nil) = %v, want nil", err)
	}
	if buf.Len() == 0 {
		t.Fatal("verbose Counter(nil) wrote nothing, want metric line")
	}

	var v map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &v); err != nil {
		t.Fatalf("decode metric line: %v", err)
	}
	if v["name"] != "c" {
		t.Errorf("name = %v, want c", v["name"])
	}
}

func TestSpan_endWithErrorEmitsError(t *testing.T) {
	t.Parallel()

	p, buf := openBuffered(t, nil)
	_, span := p.Tracer("s").Start(t.Context(), "op")
	span.RecordError(errors.New("boom"))
	span.End()

	v := decodeLine(t, buf)
	if v["error"] != "boom" {
		t.Errorf("error = %v, want boom", v["error"])
	}
}

func TestSpan_doubleEnd(t *testing.T) {
	t.Parallel()

	p, buf := openBuffered(t, nil)
	_, span := p.Tracer("s").Start(t.Context(), "op")
	span.End()
	span.End()

	if got := strings.Count(buf.String(), "\n"); got != 2 {
		t.Errorf("lines = %d, want 2 (End emits unconditionally)", got)
	}
}

func TestSpan_concurrentSetAttributes(t *testing.T) {
	t.Parallel()

	p, _ := openBuffered(t, nil)
	_, span := p.Tracer("s").Start(t.Context(), "op")

	const goroutines = 16

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			span.SetAttributes(observability.Int("i", i))
		}(i)
	}

	wg.Wait()
	span.End()
}

func TestNewWithWriter_nilWriter(t *testing.T) {
	t.Parallel()

	p, err := stdout.NewWithWriter(validOptions(), nil)
	if err != nil {
		t.Fatalf("NewWithWriter(nil) = %v, want nil", err)
	}
	if p == nil {
		t.Fatal("NewWithWriter(nil) = nil, want provider")
	}
}
