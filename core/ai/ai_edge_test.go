package ai

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
	var nilAI AI
	if err := Register(a, func(Options) (AI, error) { return nilAI, nil }); err != nil {
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

func TestEdgeOptionsValidate_userinfoBaseURL(t *testing.T) {
	t.Parallel()

	// Userinfo is permitted by endpoint.ValidateURL defaults.
	if err := (Options{BaseURL: "https://user@example.com/v1"}).Validate(); err != nil {
		t.Fatalf("Validate err = %v, want nil", err)
	}
}

func TestEdgeOptionsValidate_queryAndFragmentBaseURL(t *testing.T) {
	t.Parallel()

	cases := []string{
		"https://example.com/v1?q=1",
		"https://example.com/v1#frag",
	}
	for _, baseURL := range cases {
		if err := (Options{BaseURL: baseURL}).Validate(); err != nil {
			t.Errorf("Validate(%q) err = %v, want nil", baseURL, err)
		}
	}
}

func TestEdgeOptionsValidate_loopbackInsecureAllowed(t *testing.T) {
	t.Parallel()

	cases := []string{
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
	}
	for _, baseURL := range cases {
		if err := (Options{BaseURL: baseURL, AllowInsecure: true}).Validate(); err != nil {
			t.Errorf("Validate(%q, insecure) err = %v, want nil", baseURL, err)
		}
	}
}

func TestEdgeOptionsValidate_httpsNoHost(t *testing.T) {
	t.Parallel()

	err := (Options{BaseURL: "https://"}).Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "host") {
		t.Fatalf("err %q missing host reason", err.Error())
	}
}

func TestEdgeGenerate_nilContext(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	stub := &stubAI{}
	if err := Register(a, func(Options) (AI, error) { return stub, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if _, err := got.Generate(nilCtx, "m", nil, GenerateOptions{}); err != nil {
		t.Fatalf("Generate(nil ctx) err = %v, want nil", err)
	}
}

func TestEdgeStream_nilContext(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	stub := &stubAI{}
	if err := Register(a, func(Options) (AI, error) { return stub, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	ch, err := got.Stream(nilCtx, "m", nil, GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream(nil ctx) err = %v, want nil", err)
	}
	if _, ok := <-ch; !ok {
		t.Fatal("Stream channel closed empty")
	}
}

func TestEdgeEmbed_emptyInputs(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	stub := &stubAI{}
	if err := Register(a, func(Options) (AI, error) { return stub, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	vecs, err := got.Embed(t.Context(), "m", []string{}, EmbedOptions{})
	if err != nil {
		t.Fatalf("Embed err = %v", err)
	}
	if len(vecs) != 0 {
		t.Fatalf("Embed len = %d, want 0", len(vecs))
	}
}

func TestEdgeRateLimitedError_negativeRetryAfter(t *testing.T) {
	t.Parallel()

	err := RateLimitedError{RetryAfter: -1}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if err.Error() != ErrRateLimited.Error() {
		t.Fatalf("Error() = %q, want plain sentinel for negative RetryAfter", err.Error())
	}
}

func TestEdgeInvalidOptionsError_emptyReason(t *testing.T) {
	t.Parallel()

	err := InvalidOptionsError{}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.HasSuffix(err.Error(), ": ") {
		t.Fatalf("Error() = %q, want trailing sentinel with empty reason", err.Error())
	}
}

func TestEdgeDuplicateAdapterError_emptyAdapter(t *testing.T) {
	t.Parallel()

	err := DuplicateAdapterError{Adapter: Adapter("")}
	if !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("err = %v, want ErrDuplicateAdapter", err)
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("Error() = %q, want unknown adapter name", err.Error())
	}
}

func TestEdgeParseAdapter_whitespace(t *testing.T) {
	t.Parallel()

	// Any non-empty name is accepted, including whitespace.
	got, err := ParseAdapter(" ")
	if err != nil {
		t.Fatalf("ParseAdapter(\" \") err = %v, want nil", err)
	}
	if got != Adapter(" ") {
		t.Fatalf("ParseAdapter(\" \") = %v, want space", got)
	}
}

func TestEdgeAdapterString_custom(t *testing.T) {
	t.Parallel()

	if got := Adapter("custom-backend").String(); got != "custom-backend" {
		t.Fatalf("String() = %q, want custom-backend", got)
	}
}
