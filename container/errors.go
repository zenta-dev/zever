package container

import (
	"errors"
	"fmt"
)

// ErrTransactorUnsupported reports a db adapter that lacks transaction support.
var ErrTransactorUnsupported = errors.New("container: transactor unsupported")

// ErrCloseTimeout reports a service whose Close did not finish in time.
var ErrCloseTimeout = errors.New("container: close timed out")

// ErrClosePanic reports a service whose Close panicked.
var ErrClosePanic = errors.New("container: close panicked")

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

// Unwrap returns the underlying timeout error.
func (e CloseTimeoutError) Unwrap() error {
	return e.Err
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
