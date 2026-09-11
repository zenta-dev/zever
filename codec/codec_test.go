package codec

import (
	"testing"
)

type testCodec struct{}

func (testCodec) Encode(v string) ([]byte, error) {
	return []byte(v), nil
}

func (testCodec) Decode(data []byte) (string, error) {
	return string(data), nil
}

var (
	_ Encoder[string] = testCodec{}
	_ Decoder[string] = testCodec{}
	_ Codec[string]   = testCodec{}
)

func TestCodec(t *testing.T) {
	c := testCodec{}

	t.Run("Encode", func(t *testing.T) {
		got, err := c.Encode("hello")
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if string(got) != "hello" {
			t.Errorf("Encode() = %q, want %q", got, "hello")
		}
	})

	t.Run("Decode", func(t *testing.T) {
		got, err := c.Decode([]byte("hello"))
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if got != "hello" {
			t.Errorf("Decode() = %q, want %q", got, "hello")
		}
	})
}

func TestCodecEncode(t *testing.T) {
	c := testCodec{}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "ascii", in: "hello", want: "hello"},
		{name: "unicode", in: "你好，世界", want: "你好，世界"},
		{name: "whitespace", in: "  spaced  ", want: "  spaced  "},
		{name: "special-chars", in: "a\nb\tc\"d", want: "a\nb\tc\"d"},
		{name: "long", in: string(make([]byte, 1024)), want: string(make([]byte, 1024))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Encode(tt.in)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			if string(got) != tt.want {
				t.Errorf("Encode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCodecDecode(t *testing.T) {
	c := testCodec{}

	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{name: "empty", in: []byte{}, want: ""},
		{name: "nil", in: nil, want: ""},
		{name: "ascii", in: []byte("hello"), want: "hello"},
		{name: "unicode", in: []byte("你好，世界"), want: "你好，世界"},
		{name: "whitespace", in: []byte("  spaced  "), want: "  spaced  "},
		{name: "special-chars", in: []byte("a\nb\tc\"d"), want: "a\nb\tc\"d"},
		{name: "binary", in: []byte{0x00, 0x01, 0xff}, want: string([]byte{0x00, 0x01, 0xff})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Decode(tt.in)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("Decode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCodecRoundTrip(t *testing.T) {
	c := testCodec{}

	values := []string{"", "hello", "你好，世界", "a\nb\tc\"d", "  spaced  "}

	for i, in := range values {
		encoded, err := c.Encode(in)
		if err != nil {
			t.Fatalf("Encode() #%d error = %v", i, err)
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode() #%d error = %v", i, err)
		}

		if decoded != in {
			t.Errorf("RoundTrip #%d = %q, want %q", i, decoded, in)
		}
	}
}

func TestCodecNoError(t *testing.T) {
	c := testCodec{}

	if _, err := c.Encode(""); err != nil {
		t.Errorf("Encode() error = %v, want nil", err)
	}

	if _, err := c.Decode(nil); err != nil {
		t.Errorf("Decode() error = %v, want nil", err)
	}
}
