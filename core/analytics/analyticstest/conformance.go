// Package analyticstest provides the conformance kit third-party analytics adapters run to prove backend parity.
package analyticstest

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/analytics"
)

// Conformance verifies factory-built backends implement the
// analytics.Analytics contract: open/register round-trip,
// Track/Identify/Group happy paths, missing-group sentinel, context
// cancellation, and Close. Each subtest takes a fresh instance from
// factory so cases stay isolated. Tests never call time.Sleep and
// never touch the network.
//
// Documented live-creds exemption: network adapters (posthog) need a
// write key plus network, so their conformance tests skip with a
// reason and the kit runs against the log adapter or fakes in their
// own packages. The kit never asserts property-limit sentinels: limits
// come from factory-supplied Options, so bounds stay adapter-owned.
func Conformance(t *testing.T, factory func(t *testing.T) analytics.Analytics) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("TrackIdentifyGroup", func(t *testing.T) { conformanceHappyPath(t, factory) })
	t.Run("MissingGroupID", func(t *testing.T) { conformanceMissingGroupID(t, factory) })
	t.Run("CanceledContext", func(t *testing.T) { conformanceCanceledContext(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := analytics.Open(analytics.Adapter("conformance-missing-adapter"), analytics.Options{}); !errors.Is(err, analytics.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := analytics.Adapter("conformance-probe-analytics")

	if err := analytics.Register(probe, nil); !errors.Is(err, analytics.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(analytics.Options) (analytics.Analytics, error) {
		return nil, errors.New("analyticstest: probe factory must not run")
	}

	_ = analytics.Register(probe, stub)

	if err := analytics.Register(probe, stub); !errors.Is(err, analytics.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceHappyPath(t *testing.T, factory func(t *testing.T) analytics.Analytics) {
	t.Helper()

	ctx := analytics.WithUserID(t.Context(), "kit-user-01")
	a := factory(t)

	if err := a.Track(ctx, "kit_event", map[string]any{"plan": "kit"}); err != nil {
		t.Fatalf("Track() error = %v", err)
	}

	if err := a.Identify(ctx, "kit-user-01", map[string]any{"tier": "kit"}); err != nil {
		t.Fatalf("Identify() error = %v", err)
	}

	if err := a.Group(ctx, "kit-user-01", "kit-group-01", map[string]any{"seat": 1}); err != nil {
		t.Fatalf("Group() error = %v", err)
	}
}

func conformanceMissingGroupID(t *testing.T, factory func(t *testing.T) analytics.Analytics) {
	t.Helper()

	ctx := analytics.WithUserID(t.Context(), "kit-user-01")
	a := factory(t)

	if err := a.Group(ctx, "kit-user-01", "", nil); !errors.Is(err, analytics.ErrMissingGroupID) {
		t.Errorf("Group(empty group) err = %v, want ErrMissingGroupID", err)
	}
}

func conformanceCanceledContext(t *testing.T, factory func(t *testing.T) analytics.Analytics) {
	t.Helper()

	ctx := t.Context()
	a := factory(t)

	canceled, cancel := context.WithCancel(ctx)
	cancel()

	if err := a.Track(canceled, "kit_event", nil); err == nil {
		t.Error("Track(canceled) = nil, want context error")
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) analytics.Analytics) {
	t.Helper()

	a := factory(t)

	if err := a.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := a.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
