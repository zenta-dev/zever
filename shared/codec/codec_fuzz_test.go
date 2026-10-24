package codec

import (
	"bytes"
	"errors"
	"math"
	"testing"
	"time"
)

// FuzzJSONRoundTrip proves Encode never panics on arbitrary field inputs and
// that decode(marshal(x)) is stable: decoding then re-encoding yields
// byte-identical output (deterministic opts), and the second decode matches
// the first. Encode failures (invalid UTF-8, non-finite floats) fail closed
// and must surface as a typed EncodeError.
func FuzzJSONRoundTrip(f *testing.F) {
	seeds := []struct {
		name     string
		count    int
		score    float64
		itemA    string
		itemB    string
		tagKey   string
		tagValue string
	}{
		{"zever", 42, 3.14, "a", "b", "k", "v"},
		{"", 0, 0, "", "", "", ""},
		{"\u4f60\u597d\u4e16\u754c <>&\"\\", -7, -0.5, "x\ny", "z", "k", "v v"},
		{"b", math.MaxInt, math.MaxFloat64, "", "", "", ""},
		{"min", math.MinInt, math.SmallestNonzeroFloat64, "a", "", "k", ""},
		{"truncated-ish", 1, 1.5, "{invalid", `{"Name":"x"`, "k\"", "\"v"},
	}
	for _, s := range seeds {
		f.Add(s.name, s.count, s.score, s.itemA, s.itemB, s.tagKey, s.tagValue)
	}

	f.Fuzz(func(t *testing.T, name string, count int, score float64, itemA, itemB, tagKey, tagValue string) {
		t.Parallel()

		c := JSONCodec[jsonTestValue]{}
		in := jsonTestValue{
			Name:  name,
			Count: count,
			Score: score,
			Items: []string{itemA, itemB},
			Tags:  map[string]string{tagKey: tagValue},
		}

		data, err := c.Encode(in)
		if err != nil {
			// Any encode failure must still be typed: invalid UTF-8
			// (the fuzzer readily produces truncated sequences like
			// "\xe2") and non-finite floats fail closed instead of
			// panicking. A panic would fail the fuzz run outright.
			var encErr *EncodeError
			if !errors.As(err, &encErr) {
				t.Fatalf("Encode() err = %T, want *EncodeError (%v)", err, err)
			}
			if !errors.Is(err, ErrEncode) {
				t.Fatalf("Encode() err = %v, want ErrEncode", err)
			}
			return
		}

		got, err := c.Decode(data)
		if err != nil {
			t.Fatalf("Decode(Encode(x)) error = %v", err)
		}

		again, err := c.Encode(got)
		if err != nil {
			t.Fatalf("Encode(Decode(x)) error = %v", err)
		}
		if !bytes.Equal(data, again) {
			t.Fatalf("Encode not stable:\nfirst  = %s\nsecond = %s", data, again)
		}

		got2, err := c.Decode(again)
		if err != nil {
			t.Fatalf("Decode(Encode(Decode(x))) error = %v", err)
		}
		if got.Name != got2.Name || got.Count != got2.Count || got.Score != got2.Score {
			t.Fatalf("round-trip not stable: %+v vs %+v", got, got2)
		}
	})
}

// FuzzJSONDecodeMalformed proves Decode never panics on arbitrary bytes and
// that every failure is a typed DecodeError wrapping ErrDecode.
func FuzzJSONDecodeMalformed(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"Name":"zever","Count":42,"Score":3.14,"Items":["a","b"],"Tags":{"k":"v"}}`),
		[]byte(`{}`),
		[]byte(`{"Name":"x"`),
		[]byte(`{invalid`),
		[]byte(``),
		[]byte(`null`),
		[]byte(`[]`),
		[]byte(`"hello"`),
		[]byte("{\x00\xff\xfe}"),
		[]byte(`{"Name":"zever",`),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		t.Parallel()

		c := JSONCodec[jsonTestValue]{}
		_, err := c.Decode(data)
		if err == nil {
			return
		}
		var decErr *DecodeError
		if !errors.As(err, &decErr) {
			t.Fatalf("Decode(%q) err = %T, want *DecodeError (%v)", data, err, err)
		}
		if !errors.Is(err, ErrDecode) {
			t.Fatalf("Decode(%q) err = %v, want ErrDecode", data, err)
		}
	})
}

// FuzzDurationMarshal proves duration handling never panics on edge inputs:
// negative strings, max-int64 nanosecond counts, and trailing garbage. The
// string path decodes s as a raw JSON fragment for the timeout field (so
// string/number/null/malformed shapes all flow through
// durationUnmarshalers); the int64 path proves every nanosecond count
// round-trips through the canonical string form. Failures must be typed
// DecodeError values.
func FuzzDurationMarshal(f *testing.F) {
	seeds := []struct {
		raw   string
		nanos int64
	}{
		{`"30s"`, int64(30 * time.Second)},
		{`"0s"`, 0},
		{`"-5s"`, int64(-5 * time.Second)},
		{`"1h30m0s"`, int64(90 * time.Minute)},
		{`"2562047h47m16.854775807s"`, math.MaxInt64},
		{`"-2562047h47m16.854775807s"`, math.MinInt64 + 1},
		{`"30s trailing"`, int64(time.Second)},
		{`"soon"`, 0},
		{`""`, 0},
		{`null`, 0},
		{`30000000000`, int64(30 * time.Second)},
		{`-1`, -1},
		{`9223372036854775807`, math.MaxInt64},
		{`"1.5h"`, int64(90 * time.Minute)},
		{`{"interval":"1s"}`, 0},
		{`"30s" `, 0},
	}
	for _, s := range seeds {
		f.Add(s.raw, s.nanos)
	}

	f.Fuzz(func(t *testing.T, raw string, nanos int64) {
		t.Parallel()

		c := JSONCodec[jsonDurationValue]{}

		// Path A: raw fragment as the timeout value (string, number,
		// null, or malformed bytes).
		got, err := c.Decode([]byte(`{"timeout":` + raw + `}`))
		if err != nil {
			var decErr *DecodeError
			if !errors.As(err, &decErr) {
				t.Fatalf("Decode(timeout=%q) err = %T, want *DecodeError (%v)", raw, err, err)
			}
			if !errors.Is(err, ErrDecode) {
				t.Fatalf("Decode(timeout=%q) err = %v, want ErrDecode", raw, err)
			}
		} else {
			// A successful decode must be stable across re-encode.
			// Compare bytes, not DeepEqual: encoding/json/v2 normalizes
			// nil slices/maps to []/{}, so the first decode may hold
			// nils where the second holds empties.
			re, rerr := c.Encode(got)
			if rerr != nil {
				t.Fatalf("Encode(Decode(timeout=%q)) error = %v", raw, rerr)
			}
			got2, rerr := c.Decode(re)
			if rerr != nil {
				t.Fatalf("Decode(Encode(Decode(timeout=%q))) error = %v", raw, rerr)
			}
			re2, rerr := c.Encode(got2)
			if rerr != nil {
				t.Fatalf("Encode(Decode(Encode(Decode(timeout=%q)))) error = %v", raw, rerr)
			}
			if !bytes.Equal(re, re2) {
				t.Fatalf("duration round-trip not stable:\nfirst  = %s\nsecond = %s (raw=%q)", re, re2, raw)
			}
		}

		// Path B: every int64 nanosecond count round-trips.
		d := time.Duration(nanos)
		enc, err := c.Encode(jsonDurationValue{Timeout: d})
		if err != nil {
			t.Fatalf("Encode(Timeout=%d) error = %v", nanos, err)
		}
		dec, err := c.Decode(enc)
		if err != nil {
			t.Fatalf("Decode(Encode(Timeout=%d)) error = %v", nanos, err)
		}
		if dec.Timeout != d {
			t.Fatalf("duration nanos round-trip = %d, want %d", dec.Timeout, d)
		}
	})
}
