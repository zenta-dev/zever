package job

import (
	"errors"
	"fmt"
)

// ErrRegisterNameEmpty is returned when Register is called with a blank name.
var ErrRegisterNameEmpty = errors.New("job: register name is empty")

// ErrRegisterHandleNil is returned when Register is called with a nil handler.
var ErrRegisterHandleNil = errors.New("job: register handle is nil")

// ErrUseMiddlewareNil is returned when Use is called with a nil middleware.
var ErrUseMiddlewareNil = errors.New("job: use middleware is nil")

// ErrDuplicateJob is returned on duplicate job registration.
var ErrDuplicateJob = errors.New("job: duplicate registration")

// ErrUnknownJob is returned for a job name with no registered definition.
var ErrUnknownJob = errors.New("job: unknown job")

// ErrMemberLost is reported to a batch's onFailure callback when a member's
// outcome could not be determined before the batch's settlement timeout.
var ErrMemberLost = errors.New("job: batch member lost")

// ErrInvalidLockTTL is returned when a unique lock is requested with a non-positive TTL.
var ErrInvalidLockTTL = errors.New("job: unique lock ttl must be > 0")

// ErrUniqueLockerNil is returned when a unique lock or scheduled run is requested but no locker is configured.
var ErrUniqueLockerNil = errors.New("job: unique locker is nil")

// ErrHandlerPanic is returned when a job handler panics during worker execution.
var ErrHandlerPanic = errors.New("job: handler panic")

// DuplicateJobError reports a duplicate job registration.
type DuplicateJobError struct {
	// Name is the already-registered job name.
	Name string
}

// Error returns a human-readable duplicate-registration message.
func (e DuplicateJobError) Error() string {
	return fmt.Sprintf("%s: %q", ErrDuplicateJob, e.Name)
}

// Unwrap returns ErrDuplicateJob.
func (e DuplicateJobError) Unwrap() error {
	return ErrDuplicateJob
}

// UnknownJobError reports a lookup or dispatch of an unregistered job.
type UnknownJobError struct {
	// Name is the unregistered job name.
	Name string
}

// Error returns a human-readable unknown-job message.
func (e UnknownJobError) Error() string {
	return fmt.Sprintf("%s: %q", ErrUnknownJob, e.Name)
}

// Unwrap returns ErrUnknownJob.
func (e UnknownJobError) Unwrap() error {
	return ErrUnknownJob
}
