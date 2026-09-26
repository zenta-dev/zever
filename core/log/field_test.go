package log

import (
	"errors"
	"math"
	"testing"
	"time"
)

func assertFieldEqual(t *testing.T, got, want Field) {
	t.Helper()

	if got.Key != want.Key {
		t.Errorf("Field.Key = %q, want %q", got.Key, want.Key)
	}

	if got.Type != want.Type {
		t.Errorf("Field.Type = %v, want %v", got.Type, want.Type)
	}

	if got.String != want.String {
		t.Errorf("Field.String = %q, want %q", got.String, want.String)
	}

	if got.Int64 != want.Int64 {
		t.Errorf("Field.Int64 = %d, want %d", got.Int64, want.Int64)
	}

	if got.Float64 != want.Float64 {
		t.Errorf("Field.Float64 = %v, want %v", got.Float64, want.Float64)
	}

	if got.Bool != want.Bool {
		t.Errorf("Field.Bool = %v, want %v", got.Bool, want.Bool)
	}

	if got.Duration != want.Duration {
		t.Errorf("Field.Duration = %v, want %v", got.Duration, want.Duration)
	}

	if !got.Time.Equal(want.Time) {
		t.Errorf("Field.Time = %v, want %v", got.Time, want.Time)
	}

	if !errors.Is(got.Err, want.Err) || (got.Err == nil) != (want.Err == nil) {
		t.Errorf("Field.Err = %v, want %v", got.Err, want.Err)
	}

	if got.Any != want.Any {
		t.Errorf("Field.Any = %#v, want %#v", got.Any, want.Any)
	}
}

func TestFieldConstructors_storeTypeAndValue(t *testing.T) {
	t.Parallel()

	now := time.Now()
	dur := 5 * time.Second
	sentinel := errors.New("boom")

	tests := []struct {
		name string
		got  Field
		want Field
	}{
		{name: "string", got: String("k", "v"), want: Field{Key: "k", Type: StringType, String: "v"}},
		{name: "string empty", got: String("", ""), want: Field{Key: "", Type: StringType}},
		{name: "int", got: Int("k", 42), want: Field{Key: "k", Type: IntType, Int64: 42}},
		{name: "int negative", got: Int("k", -1), want: Field{Key: "k", Type: IntType, Int64: -1}},
		{name: "int64 max", got: Int64("k", math.MaxInt64), want: Field{Key: "k", Type: Int64Type, Int64: math.MaxInt64}},
		{name: "int64 min", got: Int64("k", math.MinInt64), want: Field{Key: "k", Type: Int64Type, Int64: math.MinInt64}},
		{name: "float64", got: Float64("k", 3.14), want: Field{Key: "k", Type: Float64Type, Float64: 3.14}},
		{name: "float64 NaN", got: Float64("k", math.NaN()), want: Field{Key: "k", Type: Float64Type, Float64: math.NaN()}},
		{name: "bool true", got: Bool("k", true), want: Field{Key: "k", Type: BoolType, Bool: true}},
		{name: "bool false", got: Bool("k", false), want: Field{Key: "k", Type: BoolType}},
		{name: "duration", got: Duration("k", dur), want: Field{Key: "k", Type: DurationType, Duration: dur}},
		{name: "duration negative", got: Duration("k", -dur), want: Field{Key: "k", Type: DurationType, Duration: -dur}},
		{name: "time", got: Time("k", now), want: Field{Key: "k", Type: TimeType, Time: now}},
		{name: "time zero", got: Time("k", time.Time{}), want: Field{Key: "k", Type: TimeType}},
		{name: "err default key", got: Err(sentinel), want: Field{Key: "error", Type: ErrorType, Err: sentinel}},
		{name: "err nil", got: Err(nil), want: Field{Key: "error", Type: ErrorType}},
		{name: "err custom key", got: ErrKey("cause", sentinel), want: Field{Key: "cause", Type: ErrorType, Err: sentinel}},
		{name: "any string", got: Any("k", "v"), want: Field{Key: "k", Type: AnyType, Any: "v"}},
		{name: "any int", got: Any("k", 42), want: Field{Key: "k", Type: AnyType, Any: 42}},
		{name: "any nil", got: Any("k", nil), want: Field{Key: "k", Type: AnyType}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// NaN != NaN, so compare the bit pattern instead of going through the helper.
			if tt.name == "float64 NaN" {
				if got := tt.got.Float64; !(got != got) {
					t.Errorf("Float64(NaN).Float64 = %v, want NaN", got)
				}

				if tt.got.Type != Float64Type || tt.got.Key != "k" {
					t.Errorf("Field() = %+v, want key k type Float64Type", tt.got)
				}

				return
			}

			assertFieldEqual(t, tt.got, tt.want)
		})
	}
}
