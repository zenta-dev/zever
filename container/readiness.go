package container

import (
	"context"
	"errors"
	"fmt"
)

// Ready reports whether this process should receive traffic: nil means ready,
// otherwise the reasons are joined into one error. It is the single readiness
// aggregate behind the scaffolded /readyz probe and the gRPC health status,
// so every transport answers from the same decision instead of each handler
// re-deriving "is the database up?".
//
// The database is always checked: it must resolve and answer a ping. Callers
// bound the probe by passing a deadline-bearing ctx (the scaffold uses a 3s
// timeout).
//
// The outbox relay is checked only when outbox.stall_readiness is set. That
// check is best-effort by design: a store that fails to resolve is skipped
// rather than failing readiness, so turning the flag on cannot make a process
// that never wired the outbox permanently unready. Store.Status carries no
// ctx, so its own adapter-level operation timeout bounds the check.
func (c *Container) Ready(ctx context.Context) error {
	var errs []error

	if err := c.readyDatabase(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := c.readyOutbox(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// readyDatabase resolves the db service and pings it. A service that cannot
// resolve fails readiness too: a process whose database is missing cannot
// serve traffic either way.
func (c *Container) readyDatabase(ctx context.Context) error {
	d, err := c.DB()
	if err != nil {
		return DatabaseUnavailableError{Err: fmt.Errorf("resolve db: %w", err)}
	}

	if err := d.Ping(ctx); err != nil {
		return DatabaseUnavailableError{Err: err}
	}

	return nil
}

// readyOutbox applies the opt-in stall gate. It returns nil unless the flag is
// set and the resolved relay reports Status().Stalled.
func (c *Container) readyOutbox() error {
	if !c.cfg.Outbox.Options.StallReadiness {
		return nil
	}

	// A store that fails to resolve is a missing signal, not a readiness
	// failure, so the resolution error is deliberately dropped.
	store, err := c.Outbox()
	if err == nil {
		st := store.Status()
		if st.Stalled {
			return RelayStalledError{Status: st}
		}
	}

	return nil
}
