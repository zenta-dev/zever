package queue

import (
	"errors"
	"fmt"
)

// ErrClosed is returned when the queue is closed.
var ErrClosed = errors.New("queue: closed")

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("queue: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("queue: duplicate registration")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("queue: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("queue: invalid adapter")

// ErrEmpty is returned when a Pop finds no message.
var ErrEmpty = errors.New("queue: empty")

// ErrInvalidMessageID is returned for an invalid message ID.
var ErrInvalidMessageID = errors.New("queue: invalid message id")

// EmptyError reports an empty queue, optionally for a specific topic.
type EmptyError struct {
	// Topic is the empty topic, if known.
	Topic string
}

// Error returns a human-readable empty-queue message.
func (e *EmptyError) Error() string {
	if e.Topic != "" {
		return fmt.Sprintf("%s: topic %q", ErrEmpty, e.Topic)
	}
	return ErrEmpty.Error()
}

// Unwrap returns ErrEmpty.
func (e *EmptyError) Unwrap() error { return ErrEmpty }

// InvalidMessageIDError reports a failure to parse a message ID.
type InvalidMessageIDError struct {
	// ID is the invalid message ID string.
	ID string
	// Err is the underlying parse cause.
	Err error
}

// Error returns a human-readable invalid-message-ID message.
func (e *InvalidMessageIDError) Error() string {
	return fmt.Sprintf("%s %q: %v", ErrInvalidMessageID, e.ID, e.Err)
}

// Unwrap returns ErrInvalidMessageID and the cause for errors.Is/As.
func (e *InvalidMessageIDError) Unwrap() []error {
	if e.Err != nil {
		return []error{ErrInvalidMessageID, e.Err}
	}
	return []error{ErrInvalidMessageID}
}

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

// DuplicateError reports a duplicate adapter registration.
type DuplicateError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// Error returns a human-readable duplicate-registration message.
func (e DuplicateError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter.String())
}

// Unwrap returns ErrDuplicateAdapter.
func (e DuplicateError) Unwrap() error {
	return ErrDuplicateAdapter
}

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter.String())
}

// Unwrap returns ErrUnknownAdapter.
func (e UnknownAdapterError) Unwrap() error {
	return ErrUnknownAdapter
}
