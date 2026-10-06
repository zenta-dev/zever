package httpclient

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// TestReadLimited_boundaries covers empty bodies, exact limits, and clamping.
func TestReadLimited_boundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    []byte
		limit   int64
		wantLen int
		wantErr error
	}{
		{name: "empty body", data: nil, limit: 0, wantLen: 0},
		{name: "exact limit", data: []byte("abcde"), limit: 5, wantLen: 5},
		{name: "one over limit", data: []byte("abcdef"), limit: 5, wantErr: ErrTooLarge},
		{name: "negative limit clamps to zero", data: []byte{}, limit: -1, wantLen: 0},
		{name: "negative limit nonempty", data: []byte("a"), limit: -1, wantErr: ErrTooLarge},
		{name: "zero limit empty", data: []byte{}, limit: 0, wantLen: 0},
		{name: "zero limit nonempty", data: []byte("a"), limit: 0, wantErr: ErrTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ReadLimited(t.Context(), bytes.NewReader(tt.data), tt.limit)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ReadLimited() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ReadLimited() error = %v, want nil", err)
			}

			if len(got) != tt.wantLen {
				t.Fatalf("ReadLimited() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

// TestReadLimited_cancelledContext verifies a cancelled context aborts the read.
func TestReadLimited_cancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := ReadLimited(ctx, bytes.NewReader([]byte("data")), 16)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadLimited(cancelled) error = %v, want context.Canceled", err)
	}
}

// TestTooLargeError_unwrapMatchesSentinel verifies errors.Is resolves to
// ErrTooLarge through Unwrap.
func TestTooLargeError_unwrapMatchesSentinel(t *testing.T) {
	t.Parallel()

	e := &TooLargeError{Limit: 10, Size: 11}
	if !errors.Is(e, ErrTooLarge) {
		t.Error("errors.Is(e, ErrTooLarge) = false")
	}
}

// TestTooLargeError_asResolvesThroughFmtWrapping verifies a wrapped
// TooLargeError is still discoverable via errors.As.
func TestTooLargeError_asResolvesThroughFmtWrapping(t *testing.T) {
	t.Parallel()

	original := &TooLargeError{Limit: 10, Size: 11}
	wrapped := errors.Join(errors.New("read failed"), original)

	var target *TooLargeError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As(wrapped, *TooLargeError) = false")
	}

	if target.Limit != 10 || target.Size != 11 {
		t.Errorf("target = %+v, want Limit=10 Size=11", target)
	}
}

// TestWithSafeDial_blocksPrivateByDefault verifies the guard refuses a
// private address with ErrPrivateAddress.
func TestWithSafeDial_blocksPrivateByDefault(t *testing.T) {
	t.Parallel()

	c := NewClient(time.Second, WithSafeDial(false))
	tr, ok := baseTransport(c).(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}

	_, err := tr.DialContext(t.Context(), "tcp", "127.0.0.1:80")
	if !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("DialContext() error = %v, want ErrPrivateAddress", err)
	}
}

// TestWithSafeDial_allowPrivateTrue verifies the guard permits private
// addresses, so the refusal sentinel is not returned.
func TestWithSafeDial_allowPrivateTrue(t *testing.T) {
	t.Parallel()

	c := NewClient(time.Second, WithSafeDial(true))
	tr, ok := baseTransport(c).(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}

	_, err := tr.DialContext(t.Context(), "tcp", "127.0.0.1:80")
	if errors.Is(err, ErrPrivateAddress) {
		t.Fatal("DialContext() refused private address despite allowPrivate=true")
	}
}

// TestWithNoRedirect_direct verifies the option installs a redirect
// policy returning ErrUseLastResponse.
func TestWithNoRedirect_direct(t *testing.T) {
	t.Parallel()

	c := NewClient(time.Second, WithNoRedirect())
	if c.CheckRedirect == nil {
		t.Fatal("CheckRedirect nil")
	}

	if err := c.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect() = %v, want ErrUseLastResponse", err)
	}
}
