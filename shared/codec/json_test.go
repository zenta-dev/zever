package codec

import (
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"

	"encoding/json/jsontext"
	"encoding/json/v2"
)

type jsonTestValue struct {
	Name  string
	Count int
	Score float64
	Items []string
	Tags  map[string]string
}

type jsonTaggedValue struct {
	Secret string `json:"-"`
	Nick   string `json:",omitempty"`
	Count  int    `json:"count,omitempty"`
}

type jsonNestedValue struct {
	Title string
	Inner jsonTestValue
	Refs  []jsonTestValue
}

var (
	_ Encoder[jsonTestValue] = JSONCodec[jsonTestValue]{}
	_ Decoder[jsonTestValue] = JSONCodec[jsonTestValue]{}
	_ Codec[jsonTestValue]   = JSONCodec[jsonTestValue]{}

	_ Encoder[string] = JSONCodec[string]{}
	_ Decoder[string] = JSONCodec[string]{}
	_ Codec[string]   = JSONCodec[string]{}

	_ Encoder[int] = JSONCodec[int]{}
	_ Decoder[int] = JSONCodec[int]{}
	_ Codec[int]   = JSONCodec[int]{}

	_ Encoder[bool] = JSONCodec[bool]{}
	_ Decoder[bool] = JSONCodec[bool]{}
	_ Codec[bool]   = JSONCodec[bool]{}

	_ Encoder[[]string]        = JSONCodec[[]string]{}
	_ Decoder[[]string]        = JSONCodec[[]string]{}
	_ Encoder[map[string]int]  = JSONCodec[map[string]int]{}
	_ Decoder[map[string]int]  = JSONCodec[map[string]int]{}
	_ Encoder[*jsonTestValue]  = JSONCodec[*jsonTestValue]{}
	_ Decoder[*jsonTestValue]  = JSONCodec[*jsonTestValue]{}
	_ Encoder[jsonNestedValue] = JSONCodec[jsonNestedValue]{}
	_ Decoder[jsonNestedValue] = JSONCodec[jsonNestedValue]{}
	_ Encoder[jsonTaggedValue] = JSONCodec[jsonTaggedValue]{}
	_ Decoder[jsonTaggedValue] = JSONCodec[jsonTaggedValue]{}
	_ Encoder[float64]         = JSONCodec[float64]{}
	_ Decoder[float64]         = JSONCodec[float64]{}
	_ Encoder[chan int]        = JSONCodec[chan int]{}
	_ Encoder[func()]          = JSONCodec[func()]{}
)

func assertSemanticError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("Decode()/Encode() error = nil, want *json.SemanticError")
	}

	var semErr *json.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("error type = %T, want *json.SemanticError (err = %v)", err, err)
	}
}

func assertSyntacticError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("Decode() error = nil, want *jsontext.SyntacticError")
	}

	var synErr *jsontext.SyntacticError
	if !errors.As(err, &synErr) {
		t.Fatalf("error type = %T, want *jsontext.SyntacticError (err = %v)", err, err)
	}
}

func TestJSONCodecEncode(t *testing.T) {
	t.Parallel()

	c := JSONCodec[jsonTestValue]{}

	tests := []struct {
		name string
		in   jsonTestValue
		want string
	}{
		{
			name: "empty",
			in:   jsonTestValue{},
			want: `{"Name":"","Count":0,"Score":0,"Items":[],"Tags":{}}`,
		},
		{
			name: "populated",
			in: jsonTestValue{
				Name:  "zever",
				Count: 42,
				Score: 3.14,
				Items: []string{"a", "b"},
				Tags:  map[string]string{"k": "v"},
			},
			want: `{"Name":"zever","Count":42,"Score":3.14,"Items":["a","b"],"Tags":{"k":"v"}}`,
		},
		{
			name: "nil slices and maps encode as empty",
			in:   jsonTestValue{Name: "x", Items: nil, Tags: nil},
			want: `{"Name":"x","Count":0,"Score":0,"Items":[],"Tags":{}}`,
		},
		{
			name: "empty non-nil slices and maps match nil encoding",
			in:   jsonTestValue{Items: []string{}, Tags: map[string]string{}},
			want: `{"Name":"","Count":0,"Score":0,"Items":[],"Tags":{}}`,
		},
		{
			name: "unicode passthrough",
			in:   jsonTestValue{Name: "你好世界 <>&"},
			want: `{"Name":"你好世界 <>&","Count":0,"Score":0,"Items":[],"Tags":{}}`,
		},
		{
			name: "max int boundary",
			in:   jsonTestValue{Count: math.MaxInt},
			want: `{"Name":"","Count":9223372036854775807,"Score":0,"Items":[],"Tags":{}}`,
		},
		{
			name: "min int boundary",
			in:   jsonTestValue{Count: math.MinInt},
			want: `{"Name":"","Count":-9223372036854775808,"Score":0,"Items":[],"Tags":{}}`,
		},
		{
			name: "negative count",
			in:   jsonTestValue{Count: -1},
			want: `{"Name":"","Count":-1,"Score":0,"Items":[],"Tags":{}}`,
		},
		{
			name: "max float boundary",
			in:   jsonTestValue{Score: math.MaxFloat64},
			want: `{"Name":"","Count":0,"Score":1.7976931348623157e+308,"Items":[],"Tags":{}}`,
		},
		{
			name: "negative float",
			in:   jsonTestValue{Score: -3.14},
			want: `{"Name":"","Count":0,"Score":-3.14,"Items":[],"Tags":{}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := c.Encode(tt.in)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			if string(got) != tt.want {
				t.Errorf("Encode() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestJSONCodecDecode(t *testing.T) {
	t.Parallel()

	c := JSONCodec[jsonTestValue]{}

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		data := []byte(`{"Name":"zever","Count":42,"Score":3.14,"Items":["a","b"],"Tags":{"k":"v"}}`)

		want := jsonTestValue{
			Name:  "zever",
			Count: 42,
			Score: 3.14,
			Items: []string{"a", "b"},
			Tags:  map[string]string{"k": "v"},
		}

		got, err := c.Decode(data)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("Decode() = %+v, want %+v", got, want)
		}
	})

	t.Run("missing fields use zero values", func(t *testing.T) {
		t.Parallel()

		got, err := c.Decode([]byte(`{}`))
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if !reflect.DeepEqual(got, jsonTestValue{}) {
			t.Errorf("Decode() = %+v, want zero value", got)
		}
	})

	t.Run("unexpected extra fields ignored", func(t *testing.T) {
		t.Parallel()

		got, err := c.Decode([]byte(`{"Name":"zever","Unknown":true}`))
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if got.Name != "zever" {
			t.Errorf("Decode().Name = %q, want %q", got.Name, "zever")
		}
	})

	t.Run("null yields zero value without error", func(t *testing.T) {
		t.Parallel()

		got, err := c.Decode([]byte(`null`))
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if !reflect.DeepEqual(got, jsonTestValue{}) {
			t.Errorf("Decode(null) = %+v, want zero value", got)
		}
	})

	t.Run("invalid syntax", func(t *testing.T) {
		t.Parallel()

		_, err := c.Decode([]byte(`{invalid`))
		assertSyntacticError(t, err)
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()

		_, err := c.Decode(nil)
		assertSyntacticError(t, err)
	})

	t.Run("empty bytes", func(t *testing.T) {
		t.Parallel()

		_, err := c.Decode([]byte{})
		assertSyntacticError(t, err)
	})

	t.Run("truncated object", func(t *testing.T) {
		t.Parallel()

		_, err := c.Decode([]byte(`{"Name":"x"`))
		assertSyntacticError(t, err)
	})
}

func TestJSONCodecDecode_invalid_returnsTypedError(t *testing.T) {
	t.Parallel()

	c := JSONCodec[jsonTestValue]{}

	tests := []struct {
		name    string
		in      []byte
		assert  func(t *testing.T, err error)
		wantErr bool
	}{
		{name: "invalid syntax", in: []byte(`{invalid`), assert: assertSyntacticError, wantErr: true},
		{name: "nil input", in: nil, assert: assertSyntacticError, wantErr: true},
		{name: "empty input", in: []byte{}, assert: assertSyntacticError, wantErr: true},
		{name: "truncated", in: []byte(`{"Name":"zever",`), assert: assertSyntacticError, wantErr: true},
		{name: "array into struct", in: []byte(`[]`), assert: assertSemanticError, wantErr: true},
		{name: "string into struct", in: []byte(`"hello"`), assert: assertSemanticError, wantErr: true},
		{name: "number into struct", in: []byte(`42`), assert: assertSemanticError, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := c.Decode(tt.in)
			if !tt.wantErr {
				t.Fatalf("Decode() error = %v, want nil", err)
			}

			tt.assert(t, err)
		})
	}
}

func TestJSONCodecRoundTrip(t *testing.T) {
	t.Parallel()

	c := JSONCodec[jsonTestValue]{}

	values := []struct {
		name string
		in   jsonTestValue
	}{
		{name: "zero", in: jsonTestValue{}},
		{name: "populated", in: jsonTestValue{Name: "zever", Count: 42, Score: 3.14, Items: []string{"a", "b"}, Tags: map[string]string{"k": "v"}}},
		{name: "nil slices and maps", in: jsonTestValue{Name: "nil", Items: nil, Tags: nil}},
		{name: "unicode", in: jsonTestValue{Name: "你好，世界 <>&\"\\", Count: -7, Score: -0.5, Items: []string{"x\ny", "z"}, Tags: map[string]string{"k": "v v"}}},
		{name: "boundaries", in: jsonTestValue{Name: "b", Count: math.MaxInt, Score: math.MaxFloat64, Items: []string{}, Tags: map[string]string{}}},
		{name: "min int", in: jsonTestValue{Count: math.MinInt}},
	}

	for _, tt := range values {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := c.Encode(tt.in)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			decoded, err := c.Decode(encoded)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			// encoding/json/v2 normalizes nil slices to [] and nil maps to {},
			// so normalize the want value the same way before comparing.
			want := tt.in
			if want.Items == nil {
				want.Items = []string{}
			}
			if want.Tags == nil {
				want.Tags = map[string]string{}
			}
			// Empty non-nil and nil both decode to empty non-nil.
			if decoded.Items == nil {
				decoded.Items = []string{}
			}
			if decoded.Tags == nil {
				decoded.Tags = map[string]string{}
			}

			if !reflect.DeepEqual(decoded, want) {
				t.Errorf("RoundTrip() = %+v, want %+v", decoded, want)
			}
		})
	}
}

func TestJSONCodecScalars(t *testing.T) {
	t.Parallel()

	t.Run("string", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[string]{}

		encoded, err := c.Encode("hello")
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if string(encoded) != `"hello"` {
			t.Errorf("Encode() = %s, want %q", encoded, `"hello"`)
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if decoded != "hello" {
			t.Errorf("Decode() = %q, want %q", decoded, "hello")
		}
	})

	t.Run("int", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[int]{}

		encoded, err := c.Encode(42)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if string(encoded) != "42" {
			t.Errorf("Encode() = %s, want %s", encoded, "42")
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if decoded != 42 {
			t.Errorf("Decode() = %d, want %d", decoded, 42)
		}
	})

	t.Run("bool", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[bool]{}

		encoded, err := c.Encode(true)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if string(encoded) != "true" {
			t.Errorf("Encode() = %s, want %s", encoded, "true")
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if !decoded {
			t.Error("Decode() = false, want true")
		}
	})

	t.Run("float", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[float64]{}

		encoded, err := c.Encode(3.14)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if string(encoded) != "3.14" {
			t.Errorf("Encode() = %s, want 3.14", encoded)
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if decoded != 3.14 {
			t.Errorf("Decode() = %v, want 3.14", decoded)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[string]{}

		encoded, err := c.Encode("")
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if string(encoded) != `""` {
			t.Errorf("Encode() = %s, want %q", encoded, `""`)
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if decoded != "" {
			t.Errorf("Decode() = %q, want empty", decoded)
		}
	})

	t.Run("zero int", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[int]{}

		encoded, err := c.Encode(0)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if string(encoded) != "0" {
			t.Errorf("Encode() = %s, want 0", encoded)
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if decoded != 0 {
			t.Errorf("Decode() = %d, want 0", decoded)
		}
	})

	t.Run("null into scalar yields zero without error", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[int]{}

		got, err := c.Decode([]byte(`null`))
		if err != nil {
			t.Fatalf("Decode(null) error = %v", err)
		}

		if got != 0 {
			t.Errorf("Decode(null) = %d, want 0", got)
		}
	})
}

func TestJSONCodecEncodeUnsupported(t *testing.T) {
	t.Parallel()

	t.Run("chan", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[chan int]{}

		_, err := c.Encode(make(chan int))
		assertSemanticError(t, err)
	})

	t.Run("func", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[func()]{}

		_, err := c.Encode(func() {})
		assertSemanticError(t, err)
	})

	t.Run("NaN", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[float64]{}

		_, err := c.Encode(math.NaN())
		assertSemanticError(t, err)
	})

	t.Run("positive infinity", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[float64]{}

		_, err := c.Encode(math.Inf(1))
		assertSemanticError(t, err)
	})

	t.Run("negative infinity", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[float64]{}

		_, err := c.Encode(math.Inf(-1))
		assertSemanticError(t, err)
	})
}

func TestJSONCodecDecodeTypeMismatch(t *testing.T) {
	t.Parallel()

	c := JSONCodec[int]{}

	_, err := c.Decode([]byte(`"not an int"`))
	assertSemanticError(t, err)

	t.Run("overflow", func(t *testing.T) {
		t.Parallel()

		_, err := c.Decode([]byte(`99999999999999999999999`))
		assertSemanticError(t, err)
	})

	t.Run("null is not a mismatch", func(t *testing.T) {
		t.Parallel()

		got, err := c.Decode([]byte(`null`))
		if err != nil {
			t.Fatalf("Decode(null) error = %v", err)
		}

		if got != 0 {
			t.Errorf("Decode(null) = %d, want 0", got)
		}
	})

	t.Run("bool into int", func(t *testing.T) {
		t.Parallel()

		_, err := c.Decode([]byte(`true`))
		assertSemanticError(t, err)
	})
}

func TestJSONCodecTags_respectsStructTags(t *testing.T) {
	t.Parallel()

	c := JSONCodec[jsonTaggedValue]{}

	t.Run("encode omits ignored and empty fields", func(t *testing.T) {
		t.Parallel()

		got, err := c.Encode(jsonTaggedValue{})
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		// omitempty does not omit the number 0 in encoding/json/v2,
		// only null/empty string/empty object/empty array.
		if string(got) != `{"count":0}` {
			t.Errorf("Encode() = %s, want %s", got, `{"count":0}`)
		}
	})

	t.Run("encode populated", func(t *testing.T) {
		t.Parallel()

		got, err := c.Encode(jsonTaggedValue{Secret: "s", Nick: "n", Count: 3})
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if string(got) != `{"Nick":"n","count":3}` {
			t.Errorf("Encode() = %s, want %s", got, `{"Nick":"n","count":3}`)
		}
	})

	t.Run("decode ignores secret field", func(t *testing.T) {
		t.Parallel()

		got, err := c.Decode([]byte(`{"Secret":"x","Nick":"n","count":3}`))
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		want := jsonTaggedValue{Nick: "n", Count: 3}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Decode() = %+v, want %+v", got, want)
		}
	})
}

func TestJSONCodecGenericContainers_roundTrip(t *testing.T) {
	t.Parallel()

	t.Run("slice", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[[]string]{}

		tests := []struct {
			name string
			in   []string
		}{
			{name: "nil", in: nil},
			{name: "empty", in: []string{}},
			{name: "populated", in: []string{"a", "b", "c"}},
			{name: "unicode", in: []string{"你好", ""}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				encoded, err := c.Encode(tt.in)
				if err != nil {
					t.Fatalf("Encode() error = %v", err)
				}

				decoded, err := c.Decode(encoded)
				if err != nil {
					t.Fatalf("Decode() error = %v", err)
				}

				if len(decoded) != len(tt.in) {
					t.Fatalf("RoundTrip() len = %d, want %d", len(decoded), len(tt.in))
				}

				if !reflect.DeepEqual(decoded, append([]string{}, tt.in...)) {
					t.Errorf("RoundTrip() = %#v, want %#v", decoded, tt.in)
				}
			})
		}
	})

	t.Run("map", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[map[string]int]{}

		in := map[string]int{"a": 1, "b": 2}

		encoded, err := c.Encode(in)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if !reflect.DeepEqual(decoded, in) {
			t.Errorf("RoundTrip() = %#v, want %#v", decoded, in)
		}
	})

	t.Run("nil map encodes as empty object", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[map[string]int]{}

		got, err := c.Encode(nil)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if string(got) != `{}` {
			t.Errorf("Encode(nil) = %s, want {}", got)
		}
	})

	t.Run("pointer", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[*jsonTestValue]{}

		in := &jsonTestValue{Name: "ptr", Count: 1, Items: []string{}, Tags: map[string]string{}}

		encoded, err := c.Encode(in)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if !reflect.DeepEqual(decoded, in) {
			t.Errorf("RoundTrip() = %+v, want %+v", decoded, in)
		}
	})

	t.Run("nested struct", func(t *testing.T) {
		t.Parallel()

		c := JSONCodec[jsonNestedValue]{}

		in := jsonNestedValue{
			Title: "outer",
			Inner: jsonTestValue{Name: "inner", Count: 1, Items: []string{}, Tags: map[string]string{}},
			Refs: []jsonTestValue{
				{Name: "r1", Items: []string{}, Tags: map[string]string{}},
				{Name: "r2", Count: 2, Items: []string{}, Tags: map[string]string{}},
			},
		}

		encoded, err := c.Encode(in)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if !reflect.DeepEqual(decoded, in) {
			t.Errorf("RoundTrip() = %+v, want %+v", decoded, in)
		}
	})
}

func TestJSONCodec_concurrent_safe(t *testing.T) {
	t.Parallel()

	c := JSONCodec[jsonTestValue]{}
	in := jsonTestValue{Name: "zever", Count: 42, Items: []string{"a"}, Tags: map[string]string{"k": "v"}}

	const workers = 50

	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			encoded, err := c.Encode(in)
			if err != nil {
				t.Errorf("Encode() error = %v", err)
				return
			}

			decoded, err := c.Decode(encoded)
			if err != nil {
				t.Errorf("Decode() error = %v", err)
				return
			}

			if decoded.Name != in.Name || decoded.Count != in.Count {
				t.Errorf("concurrent RoundTrip() = %+v, want %+v", decoded, in)
			}
		}()
	}

	wg.Wait()
}
