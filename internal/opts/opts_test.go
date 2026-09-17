package opts

import (
	"testing"
	"time"
)

func TestString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
		key  string
		def  string
		want string
	}{
		{"hit", map[string]any{"key": "value", "num": 42}, "key", "", "value"},
		{"missing_returnsDef", map[string]any{}, "key", "fallback", "fallback"},
		{"nonString_returnsDef", map[string]any{"num": 42}, "num", "", ""},
		{"nilMap_returnsDef", nil, "key", "def", "def"},
		{"emptyMap_returnsDef", map[string]any{}, "key", "def", "def"},
		{"emptyStringValue", map[string]any{"k": ""}, "k", "def", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := String(tt.m, tt.key, tt.def); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRequireString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		m       map[string]any
		key     string
		want    string
		wantErr bool
	}{
		{"hit", map[string]any{"key": "value"}, "key", "value", false},
		{"missing", map[string]any{"key": "value"}, "missing", "", true},
		{"empty", map[string]any{"key": ""}, "key", "", true},
		{"nonString", map[string]any{"key": 42}, "key", "", true},
		{"nilMap", nil, "key", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := RequireString(tt.m, tt.key)
			if tt.wantErr {
				if err == nil {
					t.Fatal("RequireString() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("RequireString() unexpected error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("RequireString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
		key  string
		def  int
		want int
	}{
		{"int", map[string]any{"k": 42}, "k", 0, 42},
		{"int64", map[string]any{"k": int64(42)}, "k", 0, 42},
		{"float64", map[string]any{"k": float64(42)}, "k", 0, 42},
		{"missing", map[string]any{}, "k", 99, 99},
		{"wrongType", map[string]any{"k": "str"}, "k", 99, 99},
		{"nilMap", nil, "k", 99, 99},
		{"negative", map[string]any{"k": -5}, "k", 0, -5},
		{"zero", map[string]any{"k": 0}, "k", 99, 0},
		{"bool", map[string]any{"k": true}, "k", 99, 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Int(tt.m, tt.key, tt.def); got != tt.want {
				t.Fatalf("Int() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
		key  string
		def  int64
		want int64
	}{
		{"int64", map[string]any{"k": int64(42)}, "k", 0, 42},
		{"int", map[string]any{"k": 42}, "k", 0, 42},
		{"float64", map[string]any{"k": float64(42)}, "k", 0, 42},
		{"missing", map[string]any{}, "k", 99, 99},
		{"wrongType", map[string]any{"k": "str"}, "k", 99, 99},
		{"nilMap", nil, "k", 99, 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Int64(tt.m, tt.key, tt.def); got != tt.want {
				t.Fatalf("Int64() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
		key  string
		def  float64
		want float64
	}{
		{"float64", map[string]any{"k": 3.14}, "k", 0, 3.14},
		{"int64", map[string]any{"k": int64(3)}, "k", 0, 3.0},
		{"int", map[string]any{"k": 3}, "k", 0, 3.0},
		{"missing", map[string]any{}, "k", 99, 99},
		{"wrongType", map[string]any{"k": "str"}, "k", 99, 99},
		{"nilMap", nil, "k", 99, 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Float64(tt.m, tt.key, tt.def); got != tt.want {
				t.Fatalf("Float64() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
		key  string
		def  bool
		want bool
	}{
		{"true", map[string]any{"k": true}, "k", false, true},
		{"false", map[string]any{"k": false}, "k", true, false},
		{"missing_returnsDef", map[string]any{}, "k", true, true},
		{"nilMap_returnsDef", nil, "k", true, true},
		{"int1_true", map[string]any{"k": 1}, "k", false, true},
		{"int0_false", map[string]any{"k": 0}, "k", true, false},
		{"int64_nonzero", map[string]any{"k": int64(7)}, "k", false, true},
		{"float64_zero", map[string]any{"k": float64(0)}, "k", true, false},
		{"stringTrueWords", map[string]any{"k": "yes"}, "k", false, true},
		{"stringFalseWords", map[string]any{"k": "off"}, "k", true, false},
		{"stringUnknown_returnsDef", map[string]any{"k": "maybe"}, "k", true, true},
		{"wrongType_returnsDef", map[string]any{"k": []string{"x"}}, "k", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Bool(tt.m, tt.key, tt.def); got != tt.want {
				t.Fatalf("Bool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
		key  string
		def  time.Duration
		want time.Duration
	}{
		{"duration", map[string]any{"k": 5 * time.Second}, "k", 0, 5 * time.Second},
		{"intSecs", map[string]any{"k": 10}, "k", 0, 10 * time.Second},
		{"int64Secs", map[string]any{"k": int64(10)}, "k", 0, 10 * time.Second},
		{"float64Secs", map[string]any{"k": 1.5}, "k", 0, 1500 * time.Millisecond},
		{"string", map[string]any{"k": "5s"}, "k", 0, 5 * time.Second},
		{"stringMs", map[string]any{"k": "100ms"}, "k", 0, 100 * time.Millisecond},
		{"missing", map[string]any{}, "k", 30 * time.Second, 30 * time.Second},
		{"nilMap", nil, "k", 30 * time.Second, 30 * time.Second},
		{"wrongType", map[string]any{"k": true}, "k", 30 * time.Second, 30 * time.Second},
		{"badString", map[string]any{"k": "not-a-duration"}, "k", 30 * time.Second, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Duration(tt.m, tt.key, tt.def); got != tt.want {
				t.Fatalf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringSlice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
		want []string
	}{
		{"stringSlice", map[string]any{"k": []string{"a", "b"}}, []string{"a", "b"}},
		{"anySlice_dropsNonStrings", map[string]any{"k": []any{"a", "b", 42}}, []string{"a", "b"}},
		{"missing_nil", map[string]any{}, nil},
		{"nilMap_nil", nil, nil},
		{"wrongType_nil", map[string]any{"k": "not-a-slice"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := StringSlice(tt.m, "k")
			if len(got) != len(tt.want) {
				t.Fatalf("StringSlice() = %v, want %v", got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("StringSlice() = %v, want %v", got, tt.want)
				}
			}

			if tt.want == nil && got != nil {
				t.Fatalf("StringSlice() = %v, want nil", got)
			}
		})
	}
}

func TestMap(t *testing.T) {
	t.Parallel()

	nested := map[string]any{"a": 1}

	tests := []struct {
		name    string
		m       map[string]any
		wantNil bool
	}{
		{"hit", map[string]any{"k": nested}, false},
		{"missing", map[string]any{}, true},
		{"nilMap", nil, true},
		{"wrongType", map[string]any{"k": "nope"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Map(tt.m, "k")
			if tt.wantNil {
				if got != nil {
					t.Fatalf("Map() = %v, want nil", got)
				}

				return
			}

			if len(got) != 1 || got["a"] != 1 {
				t.Fatalf("Map() = %v, want %v", got, nested)
			}
		})
	}
}

func TestStrictString(t *testing.T) {
	t.Parallel()

	got, err := StrictString("pkg", "k", "v")
	if err != nil || got != "v" {
		t.Fatalf("StrictString() = (%q, %v), want (v, nil)", got, err)
	}

	if _, err := StrictString("pkg", "k", 42); err == nil {
		t.Fatal("StrictString(int) expected error, got nil")
	}
}

func TestStrictBool(t *testing.T) {
	t.Parallel()

	got, err := StrictBool("pkg", "k", true)
	if err != nil || !got {
		t.Fatalf("StrictBool() = (%v, %v), want (true, nil)", got, err)
	}

	if _, err := StrictBool("pkg", "k", "true"); err == nil {
		t.Fatal("StrictBool(string) expected error, got nil")
	}
}

func TestStrictInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		v       any
		want    int
		wantErr bool
	}{
		{"int", 42, 42, false},
		{"int64", int64(42), 42, false},
		{"float64", float64(42), 42, false},
		{"string_err", "42", 0, true},
		{"bool_err", true, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := StrictInt("pkg", "k", tt.v)
			if tt.wantErr {
				if err == nil {
					t.Fatal("StrictInt() expected error, got nil")
				}

				return
			}

			if err != nil || got != tt.want {
				t.Fatalf("StrictInt() = (%d, %v), want (%d, nil)", got, err, tt.want)
			}
		})
	}
}

func TestStrictInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		v       any
		want    int64
		wantErr bool
	}{
		{"int64", int64(42), 42, false},
		{"int", 42, 42, false},
		{"float64", float64(42), 42, false},
		{"string_err", "42", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := StrictInt64("pkg", "k", tt.v)
			if tt.wantErr {
				if err == nil {
					t.Fatal("StrictInt() expected error, got nil")
				}

				return
			}

			if err != nil || got != tt.want {
				t.Fatalf("StrictInt64() = (%d, %v), want (%d, nil)", got, err, tt.want)
			}
		})
	}
}

func TestStrictFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		v       any
		want    float64
		wantErr bool
	}{
		{"float64", 3.5, 3.5, false},
		{"int64", int64(3), 3.0, false},
		{"int", 3, 3.0, false},
		{"string_err", "3.5", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := StrictFloat64("pkg", "k", tt.v)
			if tt.wantErr {
				if err == nil {
					t.Fatal("StrictFloat64() expected error, got nil")
				}

				return
			}

			if err != nil || got != tt.want {
				t.Fatalf("StrictFloat64() = (%v, %v), want (%v, nil)", got, err, tt.want)
			}
		})
	}
}

func TestStrictDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		v       any
		want    time.Duration
		wantErr bool
	}{
		{"duration", 5 * time.Second, 5 * time.Second, false},
		{"int_secs", 10, 10 * time.Second, false},
		{"int64_secs", int64(10), 10 * time.Second, false},
		{"float64_secs", 1.5, 1500 * time.Millisecond, false},
		{"string", "5s", 5 * time.Second, false},
		{"badString_err", "nope", 0, true},
		{"wrongType_err", true, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := StrictDuration("pkg", "k", tt.v)
			if tt.wantErr {
				if err == nil {
					t.Fatal("StrictDuration() expected error, got nil")
				}

				return
			}

			if err != nil || got != tt.want {
				t.Fatalf("StrictDuration() = (%v, %v), want (%v, nil)", got, err, tt.want)
			}
		})
	}
}

func TestStrictStringSlice(t *testing.T) {
	t.Parallel()

	got, err := StrictStringSlice("pkg", "k", []string{"a"})
	if err != nil || len(got) != 1 || got[0] != "a" {
		t.Fatalf("StrictStringSlice() = (%v, %v), want ([a], nil)", got, err)
	}

	got, err = StrictStringSlice("pkg", "k", []any{"a", "b"})
	if err != nil || len(got) != 2 {
		t.Fatalf("StrictStringSlice(any) = (%v, %v), want ([a b], nil)", got, err)
	}

	if _, err := StrictStringSlice("pkg", "k", []any{"a", 1}); err == nil {
		t.Fatal("StrictStringSlice(mixed) expected error, got nil")
	}

	if _, err := StrictStringSlice("pkg", "k", "a"); err == nil {
		t.Fatal("StrictStringSlice(string) expected error, got nil")
	}
}

func TestStrictMap(t *testing.T) {
	t.Parallel()

	nested := map[string]any{"a": 1}

	got, err := StrictMap("pkg", "k", nested)
	if err != nil || len(got) != 1 {
		t.Fatalf("StrictMap() = (%v, %v), want (%v, nil)", got, err, nested)
	}

	if _, err := StrictMap("pkg", "k", "nope"); err == nil {
		t.Fatal("StrictMap(string) expected error, got nil")
	}
}
