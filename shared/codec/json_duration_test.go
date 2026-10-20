package codec

import (
	"reflect"
	"testing"
	"time"
)

type jsonDurationValue struct {
	Timeout time.Duration `json:"timeout"`
	TTL     time.Duration `json:"ttl"`
	Nested  struct {
		Interval time.Duration `json:"interval"`
	} `json:"nested"`
	Labels []string       `json:"labels"`
	Count  int            `json:"count"`
	Hidden map[string]int `json:"hidden"`
}

// TestJSONCodecDuration_encodesCanonicalString pins the duration
// representation: a time.Duration encodes as its canonical string form
// ("30s"), the form time.ParseDuration and the config env layer accept.
// encoding/json/v2 has no default representation for time.Duration, so
// without this the whole enclosing struct fails to encode.
func TestJSONCodecDuration_encodesCanonicalString(t *testing.T) {
	t.Parallel()

	c := JSONCodec[jsonDurationValue]{}

	got, err := c.Encode(jsonDurationValue{
		Timeout: 30 * time.Second,
		TTL:     5 * time.Minute,
		Labels:  []string{"a"},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	want := `{"timeout":"30s","ttl":"5m0s","nested":{"interval":"0s"},"labels":["a"],"count":0,"hidden":{}}`
	if string(got) != want {
		t.Errorf("Encode() = %s, want %s", got, want)
	}
}

// TestJSONCodecDuration_decodesCanonicalString pins that the string form
// round-trips back into a time.Duration.
func TestJSONCodecDuration_decodesCanonicalString(t *testing.T) {
	t.Parallel()

	c := JSONCodec[jsonDurationValue]{}

	got, err := c.Decode([]byte(`{"timeout":"30s","ttl":"5m0s","nested":{"interval":"1h30m0s"}}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	want := jsonDurationValue{Timeout: 30 * time.Second, TTL: 5 * time.Minute}
	want.Nested.Interval = 90 * time.Minute

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Decode() = %+v, want %+v", got, want)
	}
}

// TestJSONCodecDuration_decodesNanosecondNumber pins that a JSON number is
// read as nanoseconds, the shape encoding/json v1 produced, so payloads
// written by older peers still decode.
func TestJSONCodecDuration_decodesNanosecondNumber(t *testing.T) {
	t.Parallel()

	c := JSONCodec[jsonDurationValue]{}

	got, err := c.Decode([]byte(`{"timeout":30000000000}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got.Timeout != 30*time.Second {
		t.Errorf("Decode().Timeout = %s, want %s", got.Timeout, 30*time.Second)
	}
}

// TestJSONCodecDuration_rejectsUnparsableString pins that a duration string
// the env layer would also reject fails closed instead of decoding to zero.
func TestJSONCodecDuration_rejectsUnparsableString(t *testing.T) {
	t.Parallel()

	c := JSONCodec[jsonDurationValue]{}

	if _, err := c.Decode([]byte(`{"timeout":"soon"}`)); err == nil {
		t.Fatal("Decode() error = nil, want a parse failure")
	}
}
