package workflow

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("workflow: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("workflow: duplicate registration")

// ErrDuplicateAdapter aliases ErrDuplicate for compatibility.
var ErrDuplicateAdapter = ErrDuplicate

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("workflow: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("workflow: invalid adapter")

// ErrInvalidOptions is returned for invalid workflow options.
var ErrInvalidOptions = errors.New("workflow: invalid options")

// ErrUnknownRun is returned for an unknown workflow run.
var ErrUnknownRun = errors.New("workflow: unknown run")

// ErrRunCompleted is returned when a workflow run is already completed.
var ErrRunCompleted = errors.New("workflow: run completed")

// ErrPendingFull is returned when a run pending buffer is full.
var ErrPendingFull = errors.New("workflow: pending full")

// ErrUnknownStep is returned for an unknown workflow step.
var ErrUnknownStep = errors.New("workflow: unknown step")

// ErrDuplicateRun is returned on duplicate workflow run creation.
var ErrDuplicateRun = errors.New("workflow: duplicate run")

// ErrUnknownQuery is returned for an unknown workflow query.
var ErrUnknownQuery = errors.New("workflow: unknown query")

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-registration message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate, e.Adapter.String())
}

// Unwrap returns ErrDuplicate.
func (e DuplicateAdapterError) Unwrap() error {
	return ErrDuplicate
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

// UnknownRunError reports a lookup of an unknown workflow run.
type UnknownRunError struct {
	// RunID is the unknown run identifier.
	RunID RunID
}

// Error returns a human-readable unknown-run message.
func (e UnknownRunError) Error() string {
	return fmt.Sprintf("%s: %q", ErrUnknownRun, e.RunID)
}

// Unwrap returns ErrUnknownRun.
func (e UnknownRunError) Unwrap() error {
	return ErrUnknownRun
}

// RunCompletedError reports an operation on an already-completed run.
type RunCompletedError struct {
	// RunID is the completed run identifier.
	RunID RunID
}

// Error returns a human-readable run-completed message.
func (e RunCompletedError) Error() string {
	return fmt.Sprintf("workflow: run %q has completed", e.RunID)
}

// Unwrap returns ErrRunCompleted.
func (e RunCompletedError) Unwrap() error {
	return ErrRunCompleted
}

// PendingFullError reports a full pending buffer for a run.
type PendingFullError struct {
	// RunID is the run whose pending buffer is full.
	RunID RunID
}

// Error returns a human-readable pending-full message.
func (e PendingFullError) Error() string {
	return fmt.Sprintf("workflow: run %q pending buffer full", e.RunID)
}

// Unwrap returns ErrPendingFull.
func (e PendingFullError) Unwrap() error {
	return ErrPendingFull
}

// UnknownStepError reports a lookup of an unknown workflow step.
type UnknownStepError struct {
	// Step is the unknown step name.
	Step string
}

// Error returns a human-readable unknown-step message.
func (e UnknownStepError) Error() string {
	return fmt.Sprintf("%s: %q", ErrUnknownStep, e.Step)
}

// Unwrap returns ErrUnknownStep.
func (e UnknownStepError) Unwrap() error {
	return ErrUnknownStep
}

// DuplicateRunError reports a duplicate workflow run creation.
type DuplicateRunError struct {
	// RunID is the duplicated run identifier.
	RunID string
}

// Error returns a human-readable duplicate-run message.
func (e DuplicateRunError) Error() string {
	return fmt.Sprintf("workflow: run %q already started (duplicate workflow ID)", e.RunID)
}

// Unwrap returns ErrDuplicateRun.
func (e DuplicateRunError) Unwrap() error {
	return ErrDuplicateRun
}

// UnknownQueryError reports a lookup of an unknown workflow query.
type UnknownQueryError struct {
	// Query is the unknown query name.
	Query string
}

// Error returns a human-readable unknown-query message.
func (e UnknownQueryError) Error() string {
	return fmt.Sprintf("%s: %q", ErrUnknownQuery, e.Query)
}

// Unwrap returns ErrUnknownQuery.
func (e UnknownQueryError) Unwrap() error {
	return ErrUnknownQuery
}

// InvalidOptionsError reports invalid workflow options.
type InvalidOptionsError struct {
	// Reason describes why the options are invalid.
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
