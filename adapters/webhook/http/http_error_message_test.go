package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/webhook"
)

// TestDeliver_namesEventAndTarget proves the delivery-failure error says
// which event went to which target: one event can be registered against
// several targets, so a bare attempt count identifies neither.
func TestDeliver_namesEventAndTarget(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	w, err := New(webhook.Options{AllowPrivateTargets: true, MaxRetries: 1})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	ctx := t.Context()

	if regErr := w.Register(ctx, "invoice.paid", srv.URL+"/hook", "s"); regErr != nil {
		t.Fatalf("Register err = %v", regErr)
	}

	deliverErr := w.Deliver(ctx, "invoice.paid", []byte(`{}`))
	if deliverErr == nil {
		t.Fatal("Deliver() error = nil, want error")
	}

	msg := deliverErr.Error()

	for _, want := range []string{
		`http: deliver event "invoice.paid" to "`,
		srv.URL + "/hook",
		"failed after 1 attempts",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("Deliver() error = %q, want substring %q", msg, want)
		}
	}

	// The message stays on one line: a joined error renders with a
	// newline, and every single-line consumer (the `zever doctor` table
	// row) would then lose the cause.
	if strings.Contains(msg, "\n") {
		t.Errorf("Deliver() error = %q, want a single-line message", msg)
	}

	if !errors.Is(deliverErr, ErrDeliveryFailed) {
		t.Errorf("errors.Is(err, ErrDeliveryFailed) = false (err = %v)", deliverErr)
	}
}

// TestDeliver_namesTargetOnBadStatus proves the status error names the
// target that answered with the bad status.
func TestDeliver_namesTargetOnBadStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)

	w, err := New(webhook.Options{AllowPrivateTargets: true, MaxRetries: 1})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	ctx := t.Context()

	if regErr := w.Register(ctx, "e", srv.URL, "s"); regErr != nil {
		t.Fatalf("Register err = %v", regErr)
	}

	deliverErr := w.Deliver(ctx, "e", []byte(`{}`))
	if deliverErr == nil {
		t.Fatal("Deliver() error = nil, want error")
	}

	if !errors.Is(deliverErr, ErrUnexpectedStatus) {
		t.Errorf("errors.Is(err, ErrUnexpectedStatus) = false (err = %v)", deliverErr)
	}

	want := `http: target "` + srv.URL + `" returned status 418`
	if !strings.Contains(deliverErr.Error(), want) {
		t.Errorf("Deliver() error = %q, want substring %q", deliverErr.Error(), want)
	}
}

// TestRegister_rejectNamesTheTarget proves the rejection error names the
// target that was refused.
func TestRegister_rejectNamesTheTarget(t *testing.T) {
	t.Parallel()

	w, err := New(webhook.Options{})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	regErr := w.Register(t.Context(), "e", "not-a-url", "s")
	if regErr == nil {
		t.Fatal("Register() error = nil, want error")
	}

	if !strings.Contains(regErr.Error(), `http: reject target "not-a-url"`) {
		t.Errorf("Register() error = %q, want the target named", regErr.Error())
	}
}

// TestRedactTarget proves a password embedded in a target URL never reaches
// an error message, while the host and path stay visible.
func TestRedactTarget(t *testing.T) {
	t.Parallel()

	cases := []struct {
		target string
		want   string
	}{
		{"https://hooks.example.com/hook", "https://hooks.example.com/hook"},
		{"https://bob:s3cret@hooks.example.com/hook", "https://bob:xxxxx@hooks.example.com/hook"},
		{"://bad url", "://bad url"},
	}

	for _, tc := range cases {
		got := redactTarget(tc.target)
		if got != tc.want {
			t.Errorf("redactTarget(%q) = %q, want %q", tc.target, got, tc.want)
		}

		if strings.Contains(got, "s3cret") {
			t.Errorf("redactTarget(%q) = %q, leaks the password", tc.target, got)
		}
	}
}
