package container

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// pingDB is a db.DB stub whose Ping outcome the test controls. It is seeded
// through lazy.get, the only same-package seam the frozen lazy contract
// exposes.
type pingDB struct {
	db.DB

	err error
}

func (p *pingDB) Ping(_ context.Context) error { return p.err }

// Close satisfies db.DB so Close(ctx) on the container drains it.
func (p *pingDB) Close(_ context.Context) error { return nil }

// stalledOutbox is an outbox.Store stub reporting a fixed relay status. It is
// seeded the same way as pingDB.
type stalledOutbox struct {
	outbox.Store

	status outbox.Status
}

func (s *stalledOutbox) Status() outbox.Status { return s.status }

// Close satisfies outbox.Store so Close(ctx) drains it.
func (s *stalledOutbox) Close() error { return nil }

// seedReadyContainer returns a container whose db and outbox are already
// resolved with the given doubles, so Ready never opens a real adapter.
func seedReadyContainer(t *testing.T, dbErr error, relay outbox.Status) *Container {
	t.Helper()

	c := New(testConfig(t))
	closeContainer(t, c)

	if _, err := c.db.get(func() (db.DB, error) { return &pingDB{err: dbErr}, nil }); err != nil {
		t.Fatalf("seeding ping db: %v", err)
	}

	if _, err := c.outbox.get(func() (outbox.Store, error) {
		return &stalledOutbox{status: relay}, nil
	}); err != nil {
		t.Fatalf("seeding outbox: %v", err)
	}

	return c
}

// TestReady covers the whole aggregate: the database is always checked, the
// stalled relay only when the opt-in flag is set.
func TestReady(t *testing.T) {
	t.Parallel()

	pingErr := errors.New("dial tcp: connection refused")

	tests := []struct {
		name     string
		dbErr    error
		relay    outbox.Status
		gate     bool
		wantErrs []error
	}{
		{
			name:  "ready when db answers and relay is idle",
			relay: outbox.Status{Pending: 3, Processed: 9},
			gate:  true,
		},
		{
			name:     "not ready when the db ping fails",
			dbErr:    pingErr,
			wantErrs: []error{ErrDatabaseUnavailable, pingErr},
		},
		{
			name:     "not ready when the relay is stalled and the gate is on",
			relay:    outbox.Status{Pending: 12, Failed: 3, Stalled: true},
			gate:     true,
			wantErrs: []error{ErrRelayStalled},
		},
		{
			name:  "ready when the relay is stalled but the gate is off",
			relay: outbox.Status{Pending: 12, Failed: 3, Stalled: true},
		},
		{
			name:     "db failure outranks a stalled relay",
			dbErr:    pingErr,
			relay:    outbox.Status{Pending: 12, Stalled: true},
			gate:     true,
			wantErrs: []error{ErrDatabaseUnavailable, ErrRelayStalled},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := seedReadyContainer(t, tt.dbErr, tt.relay)
			c.cfg.Outbox.Options.StallReadiness = tt.gate

			err := c.Ready(t.Context())

			if len(tt.wantErrs) == 0 {
				if err != nil {
					t.Fatalf("Ready() = %v, want nil", err)
				}

				return
			}

			if err == nil {
				t.Fatal("Ready() = nil, want an error")
			}

			for _, want := range tt.wantErrs {
				if !errors.Is(err, want) {
					t.Errorf("Ready() = %v, want it to match %v", err, want)
				}
			}
		})
	}
}

// TestReady_outboxResolutionFailureIsIgnored pins the best-effort contract: the
// gate is opt-in and must not turn a process whose relay cannot be opened into
// a permanently unready one.
func TestReady_outboxResolutionFailureIsIgnored(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.Outbox.Options.StallReadiness = true
	c.cfg.Outbox.Adapter = "bogus-outbox-adapter"

	if _, err := c.db.get(func() (db.DB, error) { return &pingDB{}, nil }); err != nil {
		t.Fatalf("seeding ping db: %v", err)
	}

	if err := c.Ready(t.Context()); err != nil {
		t.Fatalf("Ready() = %v, want nil for an unresolvable relay", err)
	}
}

// TestReady_classifiesErrors proves callers can branch on the typed errors
// rather than string-matching the message.
func TestReady_classifiesErrors(t *testing.T) {
	t.Parallel()

	c := seedReadyContainer(t, nil, outbox.Status{Pending: 4, Stalled: true})
	c.cfg.Outbox.Options.StallReadiness = true

	err := c.Ready(t.Context())
	if err == nil {
		t.Fatal("Ready() = nil, want a stalled-relay error")
	}

	var stalled RelayStalledError
	if !errors.As(err, &stalled) {
		t.Fatalf("Ready() = %T, want RelayStalledError", err)
	}

	if stalled.Status.Pending != 4 {
		t.Errorf("RelayStalledError.Status.Pending = %d, want 4", stalled.Status.Pending)
	}

	var unavailable DatabaseUnavailableError
	if errors.As(err, &unavailable) {
		t.Error("a healthy db must not report DatabaseUnavailableError")
	}
}

// TestReady_databaseErrorCarriesCause proves the ping error survives for
// errors.Is/As on the cause itself, not just the container sentinel.
func TestReady_databaseErrorCarriesCause(t *testing.T) {
	t.Parallel()

	pingErr := errors.New("no route to host")

	c := seedReadyContainer(t, pingErr, outbox.Status{})

	err := c.Ready(t.Context())
	if err == nil {
		t.Fatal("Ready() = nil, want a database error")
	}

	if !errors.Is(err, pingErr) {
		t.Errorf("Ready() = %v, want it to wrap the ping error", err)
	}
}
