package redis

import (
	"testing"
	"time"
)

func TestEdge_TTLModeVal(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		ttl      time.Duration
		wantMode string
		wantVal  int64
	}{
		"zero":            {ttl: 0, wantMode: "PX", wantVal: 0},
		"nanosecond":      {ttl: time.Nanosecond, wantMode: "PX", wantVal: 1},
		"sub-millisecond": {ttl: 999 * time.Microsecond, wantMode: "PX", wantVal: 1},
		"fractional-ms":   {ttl: 1500 * time.Microsecond, wantMode: "PX", wantVal: 1},
		"sub-second":      {ttl: 500 * time.Millisecond, wantMode: "PX", wantVal: 500},
		"fractional-sec":  {ttl: 1500 * time.Millisecond, wantMode: "PX", wantVal: 1500},
		"whole-second":    {ttl: time.Second, wantMode: "EX", wantVal: 1},
		"whole-minutes":   {ttl: 90 * time.Second, wantMode: "EX", wantVal: 90},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mode, val := ttlModeVal(tc.ttl)
			if mode != tc.wantMode || val != tc.wantVal {
				t.Fatalf("ttlModeVal(%v) = (%q, %d), want (%q, %d)", tc.ttl, mode, val, tc.wantMode, tc.wantVal)
			}
		})
	}
}

func TestEdge_AsBool(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in   any
		want bool
	}{
		"bool true":   {in: true, want: true},
		"bool false":  {in: false, want: false},
		"int64 one":   {in: int64(1), want: true},
		"int64 zero":  {in: int64(0), want: false},
		"int nonzero": {in: int(2), want: true},
		"string":      {in: "x", want: false},
		"nil":         {in: nil, want: false},
		"float":       {in: float64(1), want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := asBool(tc.in); got != tc.want {
				t.Fatalf("asBool(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestEdge_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	pending := encodePending([]byte("fp"))
	if len(pending) != 3+len("fp") || pending[0] != tagPending {
		t.Fatalf("encodePending = %v, want pending tag and 3-byte header", pending)
	}

	tag, fp, result, err := decode(pending)
	if err != nil {
		t.Fatalf("decode(pending) err = %v", err)
	}

	if tag != tagPending || string(fp) != "fp" || len(result) != 0 {
		t.Fatalf("decode(pending) = (%q, %q, %q), want (P, fp, empty)", tag, fp, result)
	}

	done := encodeDone([]byte("fp"), []byte("res"))
	if done[0] != tagDone {
		t.Fatalf("encodeDone tag = %q, want D", done[0])
	}

	tag, fp, result, err = decode(done)
	if err != nil {
		t.Fatalf("decode(done) err = %v", err)
	}

	if tag != tagDone || string(fp) != "fp" || string(result) != "res" {
		t.Fatalf("decode(done) = (%q, %q, %q), want (D, fp, res)", tag, fp, result)
	}
}
