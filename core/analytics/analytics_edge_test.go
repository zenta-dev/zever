package analytics

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// nilCtx is a nil context used to prove the battery never dereferences ctx
// before the adapter boundary.
var nilCtx context.Context

func TestEdgeOpen_emptyAdapterName(t *testing.T) {
	t.Parallel()

	got, err := Open(Adapter(""), Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open(\"\") err = %v, want ErrUnknownAdapter", err)
	}
	var ue UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}
	if ue.Adapter != Adapter("") {
		t.Fatalf("Adapter = %v, want empty", ue.Adapter)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestEdgeOpen_factoryReturnsNilNil(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	var nilAnalytics Analytics
	if err := Register(a, func(Options) (Analytics, error) { return nilAnalytics, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestEdgeOptionsValidate_endpointQueryAndFragment(t *testing.T) {
	t.Parallel()

	// analytics uses net/url.Parse directly; query and fragment are accepted.
	cases := []string{
		"https://example.com/track?a=b",
		"https://example.com/track#frag",
	}
	for _, endpoint := range cases {
		if err := (Options{Endpoint: endpoint}).Validate(); err != nil {
			t.Errorf("Validate(%q) err = %v, want nil", endpoint, err)
		}
	}
}

func TestEdgeOptionsValidate_endpointUnparsable(t *testing.T) {
	t.Parallel()

	err := (Options{Endpoint: "://bad"}).Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "valid url") {
		t.Fatalf("err %q missing valid url reason", err.Error())
	}
}

func TestEdgeValidatePropertiesSize_emptyMap(t *testing.T) {
	t.Parallel()

	if err := ValidatePropertiesSize(map[string]any{}, 10); err != nil {
		t.Fatalf("ValidatePropertiesSize(empty) err = %v, want nil", err)
	}
}

func TestEdgeValidatePropertiesSize_exactLimit(t *testing.T) {
	t.Parallel()

	// {"a":"b"} marshals to exactly 9 bytes.
	m := map[string]any{"a": "b"}
	if err := ValidatePropertiesSize(m, 9); err != nil {
		t.Fatalf("ValidatePropertiesSize(at limit) err = %v, want nil", err)
	}
	err := ValidatePropertiesSize(m, 8)
	var sle SizeLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("err %T is not *SizeLimitError", err)
	}
	if sle.Size != 9 || sle.Limit != 8 {
		t.Fatalf("SizeLimitError = %+v, want {Size:9 Limit:8}", sle)
	}
}

func TestEdgeMarshalValidated_emptyMap(t *testing.T) {
	t.Parallel()

	b, err := MarshalValidated(map[string]any{}, 10)
	if err != nil {
		t.Fatalf("MarshalValidated(empty) err = %v", err)
	}
	if string(b) != "{}" {
		t.Fatalf("MarshalValidated(empty) = %q, want {}", b)
	}
}

func TestEdgeValidateBounds_zeroMaxPropertiesSkipsCount(t *testing.T) {
	t.Parallel()

	// maxProperties <= 0 disables the count check; only size is enforced.
	m := map[string]any{"a": "b", "c": "d"}
	if err := ValidateBounds(m, 0, 1024); err != nil {
		t.Fatalf("ValidateBounds err = %v, want nil", err)
	}
}

func TestEdgeValidateBounds_nilMap(t *testing.T) {
	t.Parallel()

	if err := ValidateBounds(nil, 1, 1); err != nil {
		t.Fatalf("ValidateBounds(nil) err = %v, want nil", err)
	}
}

func TestEdgeTrack_nilContext(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	stub := &stubAnalytics{}
	if err := Register(a, func(Options) (Analytics, error) { return stub, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if err := got.Track(nilCtx, "evt", nil); err != nil {
		t.Fatalf("Track(nil ctx) err = %v, want nil", err)
	}
	if err := got.Identify(nilCtx, "u1", nil); err != nil {
		t.Fatalf("Identify(nil ctx) err = %v, want nil", err)
	}
	if err := got.Group(nilCtx, "u1", "g1", nil); err != nil {
		t.Fatalf("Group(nil ctx) err = %v, want nil", err)
	}
}

func TestEdgeUserIDWithFallback_emptyFallback(t *testing.T) {
	t.Parallel()

	if got := UserIDWithFallback(t.Context(), ""); got != "" {
		t.Fatalf("UserIDWithFallback(ctx, \"\") = %q, want empty", got)
	}
}

func TestEdgeCountLimitError_message(t *testing.T) {
	t.Parallel()

	err := CountLimitError{Count: 5, Limit: 3}
	if !errors.Is(err, ErrTooManyProperties) {
		t.Fatalf("err = %v, want ErrTooManyProperties", err)
	}
	if !strings.Contains(err.Error(), "count 5 exceeds limit 3") {
		t.Fatalf("Error() = %q, want count/limit detail", err.Error())
	}
}

func TestEdgeSizeLimitError_message(t *testing.T) {
	t.Parallel()

	err := SizeLimitError{Size: 100, Limit: 10}
	if !errors.Is(err, ErrPropertiesTooLarge) {
		t.Fatalf("err = %v, want ErrPropertiesTooLarge", err)
	}
	if !strings.Contains(err.Error(), "size 100 exceeds limit 10") {
		t.Fatalf("Error() = %q, want size/limit detail", err.Error())
	}
}

func TestEdgeParseAdapter_whitespace(t *testing.T) {
	t.Parallel()

	got, err := ParseAdapter(" ")
	if err != nil {
		t.Fatalf("ParseAdapter(\" \") err = %v, want nil", err)
	}
	if got != Adapter(" ") {
		t.Fatalf("ParseAdapter(\" \") = %v, want space", got)
	}
}
