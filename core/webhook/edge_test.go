package webhook

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
)

func TestValidateTargetSyntax_whitespaceAndEdges(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"leading space", " https://example.com/hook", true},
		{"trailing space in path", "https://example.com/hook ", false},
		{"empty host only", "https://", true},
		{"valid with port", "https://example.com:8443/hook", false},
		{"valid with query", "https://example.com/hook?x=1", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTargetSyntax(tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateTargetSyntax(%q) = nil, want error", tc.target)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateTargetSyntax(%q) = %v, want nil", tc.target, err)
			}
		})
	}
}

func TestValidateTargetContext_emptyTarget(t *testing.T) {
	t.Parallel()

	err := ValidateTargetContext(t.Context(), "")
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("ValidateTargetContext(empty) err = %v, want ErrInvalidTarget", err)
	}
}

func TestValidateTargetContext_cancelledCtx(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// A cancelled context surfaces as a lookup error (never a panic, never a
	// silent accept) for a hostname target.
	err := ValidateTargetContext(ctx, "https://example.com/hook")
	if err == nil {
		t.Fatal("ValidateTargetContext(cancelled ctx) = nil, want lookup error")
	}
	if !strings.Contains(err.Error(), "host lookup failed") {
		t.Fatalf("ValidateTargetContext(cancelled) err = %v, want host lookup failed", err)
	}
}

func TestIsPrivateIP_publicLiteral(t *testing.T) {
	t.Parallel()

	if IsPrivateIP(net.ParseIP("8.8.8.8")) {
		t.Error("IsPrivateIP(8.8.8.8) = true, want false")
	}
	if IsPrivateIP(net.ParseIP("2001:db8::1")) {
		t.Error("IsPrivateIP(2001:db8::1) = true, want false")
	}
}

func TestRegister_Open_concurrent(t *testing.T) {
	t.Parallel()

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			a := freshAdapter()
			if err := Register(a, func(Options) (Webhook, error) { return &stubWebhook{}, nil }); err != nil {
				t.Errorf("Register(%v) error = %v", a, err)
				return
			}
			if _, err := Open(a, Options{}); err != nil {
				t.Errorf("Open(%v) error = %v", a, err)
			}
		}()
	}

	wg.Wait()
}
