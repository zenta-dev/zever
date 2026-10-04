package observability

import (
	"strings"
	"testing"
)

func TestNormalizeAttrs_nilAndEmpty(t *testing.T) {
	t.Parallel()

	if got := normalizeAttrs(nil, MaxValueLen); len(got) != 0 {
		t.Fatalf("normalizeAttrs(nil) len = %d, want 0", len(got))
	}
	if got := normalizeAttrs([]Attr{}, MaxValueLen); len(got) != 0 {
		t.Fatalf("normalizeAttrs(empty) len = %d, want 0", len(got))
	}
}

func TestNormalizeAttrs_zeroLimit_usesDefaultValueLimit(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", MaxValueLen+10)
	got := normalizeAttrs([]Attr{String("k", long)}, 0)

	if len(got) != 1 {
		t.Fatalf("normalizeAttrs len = %d, want 1", len(got))
	}
	v, ok := got[0].Value.(StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}
	if len(v.Value) != MaxValueLen {
		t.Fatalf("truncated value len = %d, want %d", len(v.Value), MaxValueLen)
	}
}
