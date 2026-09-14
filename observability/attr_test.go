package observability

import (
	"strings"
	"testing"
)

func TestAttr_constructors_carryKeyValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		attr  Attr
		key   string
		check func(t *testing.T, v AttributeValue)
	}{
		{name: "string", attr: String("k", "v"), key: "k", check: func(t *testing.T, v AttributeValue) {
			t.Helper()
			sv, ok := v.(StringValue)
			if !ok {
				t.Fatalf("value type = %T, want StringValue", v)
			}
			if sv.Value != "v" {
				t.Errorf("value = %q, want %q", sv.Value, "v")
			}
		}},
		{name: "int", attr: Int("k", 42), key: "k", check: func(t *testing.T, v AttributeValue) {
			t.Helper()
			iv, ok := v.(Int64Value)
			if !ok {
				t.Fatalf("value type = %T, want Int64Value", v)
			}
			if iv.Value != 42 {
				t.Errorf("value = %d, want 42", iv.Value)
			}
		}},
		{name: "int64", attr: Int64("k", -7), key: "k", check: func(t *testing.T, v AttributeValue) {
			t.Helper()
			iv, ok := v.(Int64Value)
			if !ok {
				t.Fatalf("value type = %T, want Int64Value", v)
			}
			if iv.Value != -7 {
				t.Errorf("value = %d, want -7", iv.Value)
			}
		}},
		{name: "float64", attr: Float64("k", 1.5), key: "k", check: func(t *testing.T, v AttributeValue) {
			t.Helper()
			fv, ok := v.(Float64Value)
			if !ok {
				t.Fatalf("value type = %T, want Float64Value", v)
			}
			if fv.Value != 1.5 {
				t.Errorf("value = %v, want 1.5", fv.Value)
			}
		}},
		{name: "bool", attr: Bool("k", true), key: "k", check: func(t *testing.T, v AttributeValue) {
			t.Helper()
			bv, ok := v.(BoolValue)
			if !ok {
				t.Fatalf("value type = %T, want BoolValue", v)
			}
			if bv.Value != true {
				t.Errorf("value = %v, want true", bv.Value)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.attr.Key != tt.key {
				t.Errorf("Key = %q, want %q", tt.attr.Key, tt.key)
			}
			tt.check(t, tt.attr.Value)
		})
	}
}

func TestAttrValue_constructors_carryValue(t *testing.T) {
	t.Parallel()

	if got := StringAttr("v").Value; got != "v" {
		t.Errorf("StringAttr = %q, want %q", got, "v")
	}
	if got := IntAttr(3).Value; got != 3 {
		t.Errorf("IntAttr = %d, want 3", got)
	}
	if got := Int64Attr(-9).Value; got != -9 {
		t.Errorf("Int64Attr = %d, want -9", got)
	}
	if got := Float64Attr(2.5).Value; got != 2.5 {
		t.Errorf("Float64Attr = %v, want 2.5", got)
	}
	if got := BoolAttr(true).Value; got != true {
		t.Errorf("BoolAttr = %v, want true", got)
	}
}

func TestRedact_sensitiveKeys_redacted(t *testing.T) {
	t.Parallel()

	sensitive := []string{
		"password", "PASSWORD", "dbPassword",
		"secret", "Secret",
		"token", "TOKEN",
		"api_key", "API_KEY", "apikey", "APIKEY",
		"auth", "Auth",
		"cookie", "session",
		"private_key", "PRIVATE_KEY", "privatekey",
	}
	for _, key := range sensitive {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			if !redact(key) {
				t.Errorf("redact(%q) = false, want true", key)
			}
		})
	}
}

func TestRedact_benignKeys_kept(t *testing.T) {
	t.Parallel()

	benign := []string{"user", "service", "endpoint", "trace_id", "count"}
	for _, key := range benign {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			if redact(key) {
				t.Errorf("redact(%q) = true, want false", key)
			}
		})
	}
}

func TestNormalizeAttrs_redact_replacesValue(t *testing.T) {
	t.Parallel()

	got := normalizeAttrs([]Attr{String("password", "hunter2")}, 0)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	sv, ok := got[0].Value.(StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}
	if sv.Value != "[redacted]" {
		t.Errorf("value = %q, want %q", sv.Value, "[redacted]")
	}
}

func TestNormalizeAttrs_truncate_clipsLongStrings(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", MaxValueLen+100)
	got := normalizeAttrs([]Attr{String("k", long)}, 0)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	sv, ok := got[0].Value.(StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}
	if len(sv.Value) != MaxValueLen {
		t.Errorf("len = %d, want %d", len(sv.Value), MaxValueLen)
	}
}

func TestNormalizeAttrs_truncate_respectsCustomLimit(t *testing.T) {
	t.Parallel()

	got := normalizeAttrs([]Attr{String("k", "abcdef")}, 3)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	sv, ok := got[0].Value.(StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}
	if sv.Value != "abc" {
		t.Errorf("value = %q, want %q", sv.Value, "abc")
	}
}

func TestNormalizeAttrs_cap_dropsBeyondMaxAttrs(t *testing.T) {
	t.Parallel()

	in := make([]Attr, 0, MaxAttrs+5)
	for i := 0; i < MaxAttrs+5; i++ {
		in = append(in, Int("k", i))
	}
	got := normalizeAttrs(in, 0)
	if len(got) != MaxAttrs {
		t.Errorf("len = %d, want %d", len(got), MaxAttrs)
	}
}
