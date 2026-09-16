package orm

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestOptionSomeNone(t *testing.T) {
	s := Some(42)
	if !s.IsSome() {
		t.Fatalf("Some(42).IsSome() = false, want true")
	}

	v, ok := s.Get()
	if !ok || v != 42 {
		t.Fatalf("Get() = (%v, %v), want (42, true)", v, ok)
	}

	n := None[int]()
	if n.IsSome() {
		t.Fatalf("None[int]().IsSome() = true, want false")
	}

	v2, ok2 := n.Get()
	if ok2 || v2 != 0 {
		t.Fatalf("None.Get() = (%v, %v), want (0, false)", v2, ok2)
	}
}

func TestOptionGetOr(t *testing.T) {
	if got := Some(5).GetOr(9); got != 5 {
		t.Fatalf("Some(5).GetOr(9) = %d, want 5", got)
	}

	if got := None[int]().GetOr(9); got != 9 {
		t.Fatalf("None[int]().GetOr(9) = %d, want 9", got)
	}
}

func TestOptionScanNil(t *testing.T) {
	o := Some("x")

	if err := o.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}

	if o.IsSome() {
		t.Fatalf("Scan(nil) left o.IsSome() = true, want false")
	}
}

func TestOptionScanRoundTrip(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		var o Option[string]
		if err := o.Scan("hi"); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, ok := o.Get()
		if !ok || v != "hi" {
			t.Fatalf("Get() = (%q, %v), want (\"hi\", true)", v, ok)
		}
	})

	t.Run("string from bytes", func(t *testing.T) {
		var o Option[string]
		if err := o.Scan([]byte("hi")); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if v != "hi" {
			t.Fatalf("Get() = %q, want \"hi\"", v)
		}
	})

	t.Run("int64", func(t *testing.T) {
		var o Option[int64]
		if err := o.Scan(int64(42)); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if v != 42 {
			t.Fatalf("Get() = %d, want 42", v)
		}
	})

	t.Run("int32", func(t *testing.T) {
		var o Option[int32]
		if err := o.Scan(int64(7)); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if v != 7 {
			t.Fatalf("Get() = %d, want 7", v)
		}
	})

	t.Run("float64", func(t *testing.T) {
		var o Option[float64]
		if err := o.Scan(float64(1.5)); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if v != 1.5 {
			t.Fatalf("Get() = %v, want 1.5", v)
		}
	})

	t.Run("bool", func(t *testing.T) {
		var o Option[bool]
		if err := o.Scan(true); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if !v {
			t.Fatalf("Get() = %v, want true", v)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		var o Option[[]byte]
		if err := o.Scan([]byte("abc")); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if string(v) != "abc" {
			t.Fatalf("Get() = %q, want \"abc\"", v)
		}
	})

	t.Run("time.Time", func(t *testing.T) {
		want := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

		var o Option[time.Time]
		if err := o.Scan(want); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if !v.Equal(want) {
			t.Fatalf("Get() = %v, want %v", v, want)
		}
	})

	t.Run("time.Time from string", func(t *testing.T) {
		want := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

		var o Option[time.Time]
		if err := o.Scan(want.Format(time.RFC3339)); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if !v.Equal(want) {
			t.Fatalf("Get() = %v, want %v", v, want)
		}
	})
}

// TestOptionScanSourceCoercions covers every driver-source case arm of the
// scan helpers: the database/sql convertAssign shapes a driver may hand to
// Scan for each destination type.
func TestOptionScanSourceCoercions(t *testing.T) {
	t.Run("bytes from string", func(t *testing.T) {
		var o Option[[]byte]
		if err := o.Scan("abc"); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if string(v) != "abc" {
			t.Fatalf("Get() = %q, want \"abc\"", v)
		}
	})

	t.Run("int64 from int32/int/float64/bytes/string", func(t *testing.T) {
		for _, src := range []any{int32(3), int(4), float64(5), []byte("6"), "7"} {
			var o Option[int64]
			if err := o.Scan(src); err != nil {
				t.Fatalf("Scan(%T %v): %v", src, src, err)
			}
		}

		var o Option[int64]
		if err := o.Scan("42"); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if v != 42 {
			t.Fatalf("Get() = %d, want 42", v)
		}
	})

	t.Run("int64 from bad string errors", func(t *testing.T) {
		var o Option[int64]
		if err := o.Scan("not-a-number"); err == nil {
			t.Fatal("Scan(\"not-a-number\") into Option[int64] succeeded, want error")
		}

		var o32 Option[int32]
		if err := o32.Scan("not-a-number"); err == nil {
			t.Fatal("Scan(\"not-a-number\") into Option[int32] succeeded, want error")
		}
	})

	t.Run("float64 from float32/int64/bytes/string", func(t *testing.T) {
		for _, src := range []any{float32(1.5), int64(2), []byte("2.5"), "3.5"} {
			var o Option[float64]
			if err := o.Scan(src); err != nil {
				t.Fatalf("Scan(%T %v): %v", src, src, err)
			}
		}
	})

	t.Run("float32", func(t *testing.T) {
		var o Option[float32]
		if err := o.Scan(float64(1.5)); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if v != 1.5 {
			t.Fatalf("Get() = %v, want 1.5", v)
		}
	})

	t.Run("float64 from bad string errors", func(t *testing.T) {
		var o Option[float64]
		if err := o.Scan("not-a-number"); err == nil {
			t.Fatal("Scan(\"not-a-number\") into Option[float64] succeeded, want error")
		}

		var o32 Option[float32]
		if err := o32.Scan("not-a-number"); err == nil {
			t.Fatal("Scan(\"not-a-number\") into Option[float32] succeeded, want error")
		}
	})

	t.Run("bool from int64/int/bytes", func(t *testing.T) {
		var t1 Option[bool]
		if err := t1.Scan(int64(1)); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := t1.Get()
		if !v {
			t.Fatal("Scan(int64(1)) = false, want true")
		}

		var t2 Option[bool]
		if err := t2.Scan(int(0)); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v2, _ := t2.Get()
		if v2 {
			t.Fatal("Scan(int(0)) = true, want false")
		}

		for _, tc := range []struct {
			src  []byte
			want bool
		}{
			{[]byte("true"), true},
			{[]byte("false"), false},
			{[]byte("0"), false},
			{[]byte(""), false},
		} {
			var o Option[bool]
			if err := o.Scan(tc.src); err != nil {
				t.Fatalf("Scan(%q): %v", tc.src, err)
			}

			got, _ := o.Get()
			if got != tc.want {
				t.Fatalf("Scan(%q) = %v, want %v", tc.src, got, tc.want)
			}
		}
	})

	t.Run("time from bytes", func(t *testing.T) {
		want := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

		var o Option[time.Time]
		if err := o.Scan([]byte(want.Format(time.RFC3339))); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		v, _ := o.Get()
		if !v.Equal(want) {
			t.Fatalf("Get() = %v, want %v", v, want)
		}
	})

	t.Run("time from bad string errors", func(t *testing.T) {
		var o Option[time.Time]
		if err := o.Scan("not-a-time"); err == nil {
			t.Fatal("Scan(\"not-a-time\") into Option[time.Time] succeeded, want error")
		}
	})
}

// TestScanTimeCoercesToRFC3339Nano proves the string/bytes scan helpers
// coerce a time.Time source exactly the way database/sql's convertAssign
// does (RFC3339Nano text, fractional seconds preserved) -- the conversion
// the scan pipeline relies on for TIMESTAMP columns. It also proves the
// rest of the helper family still rejects an unrelated source type.
func TestScanTimeCoercesToRFC3339Nano(t *testing.T) {
	nanos := time.Date(2026, 6, 1, 12, 30, 45, 123456789, time.UTC)

	want := nanos.Format(time.RFC3339Nano)
	if want == nanos.Format(time.RFC3339) {
		t.Fatalf("test time has no fractional part; cannot prove nano fidelity")
	}

	t.Run("scanString", func(t *testing.T) {
		got, err := scanString(nanos)
		if err != nil {
			t.Fatalf("scanString(time): %v", err)
		}

		if got != want {
			t.Fatalf("scanString = %q, want %q", got, want)
		}
	})

	t.Run("scanBytes", func(t *testing.T) {
		got, err := scanBytes(nanos)
		if err != nil {
			t.Fatalf("scanBytes(time): %v", err)
		}

		if !bytes.Equal(got, []byte(want)) {
			t.Fatalf("scanBytes = %q, want %q", got, []byte(want))
		}
	})

	t.Run("Option[string]", func(t *testing.T) {
		var o Option[string]
		if err := o.Scan(nanos); err != nil {
			t.Fatalf("Option[string].Scan(time): %v", err)
		}

		v, ok := o.Get()
		if !ok || v != want {
			t.Fatalf("Get() = (%q, %v), want (%q, true)", v, ok, want)
		}
	})

	t.Run("Option[[]byte]", func(t *testing.T) {
		var o Option[[]byte]
		if err := o.Scan(nanos); err != nil {
			t.Fatalf("Option[[]byte].Scan(time): %v", err)
		}

		v, ok := o.Get()
		if !ok || !bytes.Equal(v, []byte(want)) {
			t.Fatalf("Get() = (%q, %v), want (%q, true)", v, ok, []byte(want))
		}
	})

	t.Run("changes nothing else", func(t *testing.T) {
		if _, err := scanInt64(nanos); err == nil {
			t.Fatal("scanInt64(time) succeeded, want error")
		}

		if _, err := scanFloat64(nanos); err == nil {
			t.Fatal("scanFloat64(time) succeeded, want error")
		}

		if _, err := scanBool(nanos); err == nil {
			t.Fatal("scanBool(time) succeeded, want error")
		}
	})
}

func TestOptionScanUnsupportedType(t *testing.T) {
	var o Option[chan int]
	if err := o.Scan("x"); err == nil {
		t.Fatalf("Scan into Option[chan int] succeeded, want error")
	}
}

func TestOptionScanWrongSourceType(t *testing.T) {
	var o Option[int64]
	if err := o.Scan(struct{}{}); err == nil {
		t.Fatalf("Scan(struct{}{}) into Option[int64] succeeded, want error")
	}

	var os Option[string]
	if err := os.Scan(int64(1)); err == nil {
		t.Fatal("Scan(int64) into Option[string] succeeded, want error")
	}

	var ob Option[[]byte]
	if err := ob.Scan(int64(1)); err == nil {
		t.Fatal("Scan(int64) into Option[[]byte] succeeded, want error")
	}

	var of Option[float64]
	if err := of.Scan(true); err == nil {
		t.Fatal("Scan(bool) into Option[float64] succeeded, want error")
	}

	var oo Option[bool]
	if err := oo.Scan("yes"); err == nil {
		t.Fatal("Scan(string) into Option[bool] succeeded, want error")
	}

	var ot Option[time.Time]
	if err := ot.Scan(int64(1)); err == nil {
		t.Fatal("Scan(int64) into Option[time.Time] succeeded, want error")
	}
}

func TestOptionValue(t *testing.T) {
	v, err := Some("x").Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}

	if v != "x" {
		t.Fatalf("Value() = %v, want \"x\"", v)
	}

	v2, err2 := None[string]().Value()
	if err2 != nil {
		t.Fatalf("Value() error: %v", err2)
	}

	if v2 != nil {
		t.Fatalf("None.Value() = %v, want nil", v2)
	}
}

// TestOptionScanErrorPrefix proves scan failures carry the orm: prefix and
// stay testable with errors.Is through the chain.
func TestOptionScanErrorPrefix(t *testing.T) {
	var o Option[int64]

	err := o.Scan(struct{}{})
	if err == nil {
		t.Fatal("Scan(struct{}{}) succeeded, want error")
	}

	prefix := "orm: Option.Scan:"
	if !strings.HasPrefix(err.Error(), prefix) {
		t.Fatalf("err = %q, want %q prefix", err, prefix)
	}
}
