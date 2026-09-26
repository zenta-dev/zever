package apperror

// ErrorCode is the closed, gRPC-canonical vocabulary of error codes an
// apperror error may carry. Ordinals mirror gRPC's google.rpc.Code enum
// values 1-16 (OK=0 is excluded: it isn't an error).
type ErrorCode int

const (
	// Cancelled signals an operation cancelled, typically by the caller.
	Cancelled ErrorCode = iota + 1
	// Unknown signals an error that fits no specific code and is the fail-closed default for out-of-range codes.
	Unknown
	// InvalidArgument signals a client argument invalid regardless of system state, such as a malformed file name.
	InvalidArgument
	// DeadlineExceeded signals a deadline expiring before the operation could complete, even on success delayed past the deadline.
	DeadlineExceeded
	// NotFound signals a requested entity not being found, and is preferred over FailedPrecondition when the missing entity is the cause.
	NotFound
	// AlreadyExists signals a create colliding with an entity that already exists, and is preferred over FailedPrecondition for that case.
	AlreadyExists
	// PermissionDenied signals a caller lacking permission, and must not be used for exhausted resources or unidentified callers.
	PermissionDenied
	// ResourceExhausted signals an exhausted resource, such as a per-user quota or full filesystem.
	ResourceExhausted
	// FailedPrecondition signals a system state rejecting the operation, and yields to more specific OutOfRange, NotFound, or AlreadyExists when they apply.
	FailedPrecondition
	// Aborted signals an aborted operation from a concurrency issue, such as a sequencer check failure or transaction abort.
	Aborted
	// OutOfRange signals an operation past the valid range, such as reading past end-of-file, and is preferred over FailedPrecondition as the more specific code.
	OutOfRange
	// Unimplemented signals an operation not implemented or not supported by this service.
	Unimplemented
	// Internal signals a broken system invariant reserved for serious errors.
	Internal
	// Unavailable signals a likely-transient unavailable service that the client may retry with backoff.
	Unavailable
	// DataLoss signals unrecoverable data loss or corruption.
	DataLoss
	// Unauthenticated signals missing or invalid authentication credentials for the operation.
	Unauthenticated
)

// String returns c's canonical SCREAMING_SNAKE gRPC name, e.g. "NOT_FOUND".
func (c ErrorCode) String() string {
	switch c {
	case Cancelled:
		return "CANCELLED"
	case Unknown:
		return "UNKNOWN"
	case InvalidArgument:
		return "INVALID_ARGUMENT"
	case DeadlineExceeded:
		return "DEADLINE_EXCEEDED"
	case NotFound:
		return "NOT_FOUND"
	case AlreadyExists:
		return "ALREADY_EXISTS"
	case PermissionDenied:
		return "PERMISSION_DENIED"
	case ResourceExhausted:
		return "RESOURCE_EXHAUSTED"
	case FailedPrecondition:
		return "FAILED_PRECONDITION"
	case Aborted:
		return "ABORTED"
	case OutOfRange:
		return "OUT_OF_RANGE"
	case Unimplemented:
		return "UNIMPLEMENTED"
	case Internal:
		return "INTERNAL"
	case Unavailable:
		return "UNAVAILABLE"
	case DataLoss:
		return "DATA_LOSS"
	case Unauthenticated:
		return "UNAUTHENTICATED"
	default:
		return "UNKNOWN"
	}
}

// HTTPStatus returns the canonical HTTP status for c, per Google's
// documented API error-model mapping. Several codes deliberately share a
// status (400: InvalidArgument / FailedPrecondition / OutOfRange; 409:
// AlreadyExists / Aborted; 500: Unknown / Internal / DataLoss).
func (c ErrorCode) HTTPStatus() int {
	switch c {
	case Cancelled:
		return 499
	case Unknown:
		return 500
	case InvalidArgument:
		return 400
	case DeadlineExceeded:
		return 504
	case NotFound:
		return 404
	case AlreadyExists:
		return 409
	case PermissionDenied:
		return 403
	case Unauthenticated:
		return 401
	case ResourceExhausted:
		return 429
	case FailedPrecondition:
		return 400
	case Aborted:
		return 409
	case OutOfRange:
		return 400
	case Unimplemented:
		return 501
	case Internal:
		return 500
	case Unavailable:
		return 503
	case DataLoss:
		return 500
	default:
		return 500
	}
}
