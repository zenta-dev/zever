package stdout

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/observability"
)

func validCoverOptions() observability.Options {
	return observability.Options{ServiceName: "svc", SampleRatio: 1}
}

func decodeCoverLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var v map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &v); err != nil {
		t.Fatalf("decode line %q: %v", buf.String(), err)
	}
	return v
}

func TestCoverStart_nilContext_usesBackground(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p, err := NewWithWriter(validCoverOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter() error = %v", err)
	}

	ctx, s := p.Tracer("scope").Start(nil, "op") //nolint:staticcheck // intentional nil context: exercises the nil guard in Start
	if ctx == nil {
		t.Fatal("Start(nil) ctx = nil, want non-nil")
	}
	s.End()

	if v := decodeCoverLine(t, &buf); v["name"] != "op" {
		t.Errorf("name = %v, want op", v["name"])
	}
}

func TestCoverFlatten_keyTruncation_cutsLongKeys(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("k", observability.MaxKeyLen+44)
	got := flatten([]observability.Attr{observability.String(long, "v")}, observability.MaxValueLen)

	want := long[:observability.MaxKeyLen]
	v, ok := got[want]
	if !ok {
		t.Fatalf("flatten missing truncated key, keys = %v", got)
	}
	if v != "v" {
		t.Errorf("value = %v, want v", v)
	}
}

func TestCoverPlainValue_branches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		attr  observability.Attr
		check func(t *testing.T, got any)
	}{
		{name: "int64", attr: observability.Int64("k", -7), check: func(t *testing.T, got any) {
			t.Helper()
			if got != int64(-7) {
				t.Errorf("got = %v (%T), want -7", got, got)
			}
		}},
		{name: "float64", attr: observability.Float64("k", 1.5), check: func(t *testing.T, got any) {
			t.Helper()
			if got != 1.5 {
				t.Errorf("got = %v (%T), want 1.5", got, got)
			}
		}},
		{name: "bool", attr: observability.Bool("k", true), check: func(t *testing.T, got any) {
			t.Helper()
			if got != true {
				t.Errorf("got = %v (%T), want true", got, got)
			}
		}},
		{name: "unknown", attr: observability.Attr{Key: "k"}, check: func(t *testing.T, got any) {
			t.Helper()
			if got != nil {
				t.Errorf("got = %v (%T), want nil", got, got)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.check(t, plainValue(tt.attr, observability.MaxValueLen))
		})
	}
}

func TestCoverRandHex_fallback_lengthsDistinct(t *testing.T) {
	// Not parallel: swaps the global randRead seam. randReadMu serializes the
	// swap against concurrent randHex readers in parallel span tests.
	randReadMu.Lock()
	old := randRead
	randRead = func(_ []byte) (int, error) { return 0, errors.New("no rand") }
	randReadMu.Unlock()
	defer func() {
		randReadMu.Lock()
		randRead = old
		randReadMu.Unlock()
	}()

	a16 := randHex(16)
	b16 := randHex(16)
	if len(a16) != 32 || len(b16) != 32 {
		t.Fatalf("randHex(16) lens = %d, %d, want 32, 32", len(a16), len(b16))
	}
	if a16 == b16 {
		t.Error("randHex(16) fallback values equal, want distinct")
	}

	a8 := randHex(8)
	b8 := randHex(8)
	if len(a8) != 16 || len(b8) != 16 {
		t.Fatalf("randHex(8) lens = %d, %d, want 16, 16", len(a8), len(b8))
	}
	if a8 == b8 {
		t.Error("randHex(8) fallback values equal, want distinct")
	}
}

func TestCoverNewWithWriter_invalidOptions_returnsError(t *testing.T) {
	t.Parallel()

	opts := validCoverOptions()
	opts.ServiceName = ""

	var buf bytes.Buffer
	if _, err := NewWithWriter(opts, &buf); !errors.Is(err, observability.ErrInvalidOptions) {
		t.Fatalf("NewWithWriter() err = %v, want ErrInvalidOptions", err)
	}
}

func TestCoverNewWithWriter_defaultLimit_truncatesLongAttr(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p, err := NewWithWriter(validCoverOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter() error = %v", err)
	}

	_, s := p.Tracer("scope").Start(t.Context(), "op")
	s.SetAttributes(observability.String("k", strings.Repeat("x", observability.MaxValueLen+100)))
	s.End()

	v := decodeCoverLine(t, &buf)
	attrs, ok := v["attrs"].(map[string]any)
	if !ok {
		t.Fatalf("attrs = %v, want map", v["attrs"])
	}
	sv, ok := attrs["k"].(string)
	if !ok {
		t.Fatalf("attrs[k] type = %T, want string", attrs["k"])
	}
	if len(sv) != observability.MaxValueLen {
		t.Errorf("len = %d, want %d", len(sv), observability.MaxValueLen)
	}
}

func TestCoverNewWithWriter_customLimit_respected(t *testing.T) {
	t.Parallel()

	opts := validCoverOptions()
	opts.AttrValueLimit = 256

	var buf bytes.Buffer
	p, err := NewWithWriter(opts, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter() error = %v", err)
	}

	_, s := p.Tracer("scope").Start(t.Context(), "op")
	s.SetAttributes(observability.String("k", strings.Repeat("y", 300)))
	s.End()

	v := decodeCoverLine(t, &buf)
	attrs, ok := v["attrs"].(map[string]any)
	if !ok {
		t.Fatalf("attrs = %v, want map", v["attrs"])
	}
	sv, ok := attrs["k"].(string)
	if !ok {
		t.Fatalf("attrs[k] type = %T, want string", attrs["k"])
	}
	if len(sv) != 256 {
		t.Errorf("len = %d, want %d", len(sv), 256)
	}
}
