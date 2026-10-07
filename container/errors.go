package container

import (
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/core/outbox"
)

// ErrTransactorUnsupported reports a db adapter that lacks transaction support.
var ErrTransactorUnsupported = errors.New("container: transactor unsupported")

// ErrCloseTimeout reports a service whose Close did not finish in time.
var ErrCloseTimeout = errors.New("container: close timed out")

// ErrClosePanic reports a service whose Close panicked.
var ErrClosePanic = errors.New("container: close panicked")

// ErrPluginVersionMismatch indicates a plugin version does not match.
var ErrPluginVersionMismatch = errors.New("container: plugin version mismatch")

// ErrPluginTypeMismatch indicates a plugin type does not match.
var ErrPluginTypeMismatch = errors.New("container: plugin type mismatch")

// ErrDatabaseUnavailable reports that the database did not answer a
// readiness probe.
var ErrDatabaseUnavailable = errors.New("container: database unavailable")

// ErrRelayStalled reports that the outbox relay has pending messages it is
// not draining.
var ErrRelayStalled = errors.New("container: outbox relay stalled")

// ErrPublisherSetterUnsupported reports an outbox store that cannot receive
// its destination transport after Open, so its relay cannot be started from
// the container.
var ErrPublisherSetterUnsupported = errors.New("container: outbox store does not support SetPublisher")

// TransactorError names a db adapter that does not support transactions.
type TransactorError struct {
	Actual string
}

// Error describes the adapter that lacks transaction support.
func (e TransactorError) Error() string {
	return fmt.Sprintf("container: %s does not support transactions", e.Actual)
}

// Unwrap returns ErrTransactorUnsupported.
func (e TransactorError) Unwrap() error {
	return ErrTransactorUnsupported
}

// CloseTimeoutError reports a service whose Close exceeded its deadline.
type CloseTimeoutError struct {
	Service string
	Err     error
}

// Error describes which service timed out and why.
func (e CloseTimeoutError) Error() string {
	return fmt.Sprintf("container: close %s timed out: %v", e.Service, e.Err)
}

// Unwrap returns ErrCloseTimeout joined with the underlying timeout error.
func (e CloseTimeoutError) Unwrap() error {
	if e.Err != nil {
		return errors.Join(ErrCloseTimeout, e.Err)
	}
	return ErrCloseTimeout
}

// ClosePanicError reports a service whose Close panicked.
type ClosePanicError struct {
	Service string
	Panic   any
}

// Error describes which service panicked and with what value.
func (e ClosePanicError) Error() string {
	return fmt.Sprintf("container: close %s panicked: %v", e.Service, e.Panic)
}

// Unwrap returns ErrClosePanic.
func (e ClosePanicError) Unwrap() error {
	return ErrClosePanic
}

// DatabaseUnavailableError reports a database that could not be resolved or
// did not answer a readiness ping.
type DatabaseUnavailableError struct {
	Err error
}

// Error describes the readiness failure and the underlying cause.
func (e DatabaseUnavailableError) Error() string {
	return fmt.Sprintf("container: database unavailable: %v", e.Err)
}

// Unwrap returns ErrDatabaseUnavailable joined with the underlying error, so
// errors.Is matches both the sentinel and the cause.
func (e DatabaseUnavailableError) Unwrap() error {
	if e.Err != nil {
		return errors.Join(ErrDatabaseUnavailable, e.Err)
	}
	return ErrDatabaseUnavailable
}

// RelayStalledError reports an outbox relay whose pending messages are not
// draining. It carries the relay counters so a caller can log why.
type RelayStalledError struct {
	Status outbox.Status
}

// Error describes the stalled relay and its counters.
func (e RelayStalledError) Error() string {
	return fmt.Sprintf("container: outbox relay stalled: %d pending, %d failed, last error %q",
		e.Status.Pending, e.Status.Failed, e.Status.LastError)
}

// Unwrap returns ErrRelayStalled.
func (e RelayStalledError) Unwrap() error {
	return ErrRelayStalled
}

// PublisherSetterError names an outbox store that cannot be given its
// destination transport after Open, which is what OutboxRelay needs to start
// the relay.
type PublisherSetterError struct {
	Actual string
}

// Error describes the store that does not implement outbox.PublisherSetter.
func (e PublisherSetterError) Error() string {
	return fmt.Sprintf("container: %s does not implement outbox.PublisherSetter", e.Actual)
}

// Unwrap returns ErrPublisherSetterUnsupported.
func (e PublisherSetterError) Unwrap() error {
	return ErrPublisherSetterUnsupported
}
