package db

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("db: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("db: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("db: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("db: invalid adapter")

// ErrInvalidOptions is returned for invalid database options.
var ErrInvalidOptions = errors.New("db: invalid options")

// ErrNotFound is returned when a record cannot be found.
var ErrNotFound = errors.New("db: not found")

// ErrNoSuchTable is returned when a referenced table does not exist. Adapters
// wrap this (see NoSuchTableError) so callers can use errors.Is to detect
// missing-table failures and surface actionable hints, such as the migrate
// hint in serve.
var ErrNoSuchTable = errors.New("db: no such table")

// ErrNestedTx is returned when a transaction is started inside another transaction.
var ErrNestedTx = errors.New("db: nested transaction not supported")

// ErrTxUnsupported is returned when an adapter does not support transactions.
var ErrTxUnsupported = errors.New("db: transactions not supported")

// InvalidAdapterError reports an invalid adapter name.
type InvalidAdapterError struct {
	// Adapter is the invalid adapter name.
	Adapter string
}

// Error returns a human-readable invalid-adapter message.
func (e InvalidAdapterError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidAdapter, e.Adapter)
}

// Unwrap returns ErrInvalidAdapter.
func (e InvalidAdapterError) Unwrap() error {
	return ErrInvalidAdapter
}

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter string
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-registration message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter)
}

// Unwrap returns ErrDuplicateAdapter.
func (e DuplicateAdapterError) Unwrap() error {
	return ErrDuplicateAdapter
}

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter string
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter)
}

// Unwrap returns ErrUnknownAdapter.
func (e UnknownAdapterError) Unwrap() error {
	return ErrUnknownAdapter
}

// InvalidOptionsError reports a database options validation failure.
type InvalidOptionsError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-options message.
func (e InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions.
func (e InvalidOptionsError) Unwrap() error {
	return ErrInvalidOptions
}

// TxError reports a transaction lifecycle failure.
type TxError struct {
	// Op is the failed transaction operation (begin or commit).
	Op string
	// Err is the underlying failure.
	Err error
}

// Error returns a human-readable transaction failure message.
func (e TxError) Error() string {
	return fmt.Sprintf("db: transaction %s: %v", e.Op, e.Err)
}

// Unwrap returns the underlying transaction failure.
func (e TxError) Unwrap() error {
	return e.Err
}

// NoSuchTableError reports a missing-table failure, wrapping the adapter's
// underlying error so errors.Is(err, ErrNoSuchTable) and errors.Is(err, the
// underlying cause) both match.
type NoSuchTableError struct {
	// Table is the missing table name.
	Table string
	// Err is the underlying adapter failure.
	Err error
}

// Error returns a human-readable missing-table message.
func (e *NoSuchTableError) Error() string {
	if e.Table != "" {
		return fmt.Sprintf("db: no such table: %s: %v", e.Table, e.Err)
	}

	return fmt.Sprintf("db: no such table: %v", e.Err)
}

// Unwrap returns ErrNoSuchTable and the underlying error.
func (e *NoSuchTableError) Unwrap() error {
	return errors.Join(ErrNoSuchTable, e.Err)
}
