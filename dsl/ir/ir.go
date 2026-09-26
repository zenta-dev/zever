// Package ir defines the intermediate representation (resolved schema) of the DSL compiler.
// It contains pure data types with no logic except Entity.FieldByName.
package ir

import "github.com/zenta-dev/zever/dsl/diag"

// ScalarType represents a scalar type constant in the schema.
type ScalarType int

const (
	//nolint:revive
	TUUID ScalarType = iota
	TString
	TInt32
	TInt64
	TFloat32
	TFloat64
	TBool
	TTimestamp
	TDate
	TBytes
	TJSON
	TEnum
)

// FieldType describes the type of a field, including scalar type and optional enum values.
type FieldType struct {
	Scalar     ScalarType    // The scalar type
	EnumValues []string      // EnumValues set only when Scalar==TEnum
	EnumName   string        // EnumName is set only for a named (non-anonymous) enum reference; "" for inline enum(...) values
	Pos        diag.Position // Source position of the type expression
}

// ErrorCode is the closed, gRPC-canonical vocabulary of error codes an rpc
// may declare via its errors: option. Ordinals mirror gRPC's own
// google.rpc.Code enum values 1-16 (OK=0 is excluded: it isn't an error),
// so GRPCName is trivially self-evidently correct against the real spec.
// Deliberate deviation from ScalarType's bare-enum-no-methods precedent:
// both the proto and openapi backends need the identical
// code<->HTTP-status/name mapping, and ir is their shared dependency
// (neither backend imports the other), so the methods live here instead of
// being duplicated per-backend.
type ErrorCode int

const (
	//nolint:revive
	ECancelled ErrorCode = iota + 1
	EUnknown
	EInvalidArgument
	EDeadlineExceeded
	ENotFound
	EAlreadyExists
	EPermissionDenied
	EResourceExhausted
	EFailedPrecondition
	EAborted
	EOutOfRange
	EUnimplemented
	EInternal
	EUnavailable
	EDataLoss
	EUnauthenticated
)

// HTTPStatus returns the canonical HTTP status for c, per Google's own
// documented API error-model mapping (cloud.google.com/apis/design/errors),
// the same table grpc-gateway uses. Several codes deliberately share a
// status (400: EInvalidArgument/EFailedPrecondition/EOutOfRange; 409:
// EAlreadyExists/EAborted; 500: EUnknown/EInternal/EDataLoss) — that's
// inherited from Google's own mapping, not a bug, and is why consumers that
// render one response per status (the openapi backend) must merge rather
// than overwrite.
func (c ErrorCode) HTTPStatus() int {
	switch c {
	case ECancelled:
		return 499
	case EUnknown:
		return 500
	case EInvalidArgument:
		return 400
	case EDeadlineExceeded:
		return 504
	case ENotFound:
		return 404
	case EAlreadyExists:
		return 409
	case EPermissionDenied:
		return 403
	case EUnauthenticated:
		return 401
	case EResourceExhausted:
		return 429
	case EFailedPrecondition:
		return 400
	case EAborted:
		return 409
	case EOutOfRange:
		return 400
	case EUnimplemented:
		return 501
	case EInternal:
		return 500
	case EUnavailable:
		return 503
	case EDataLoss:
		return 500
	default:
		return 500
	}
}

// GRPCName returns c's canonical SCREAMING_SNAKE gRPC name, e.g.
// "NOT_FOUND".
func (c ErrorCode) GRPCName() string {
	switch c {
	case ECancelled:
		return "CANCELLED"
	case EUnknown:
		return "UNKNOWN"
	case EInvalidArgument:
		return "INVALID_ARGUMENT"
	case EDeadlineExceeded:
		return "DEADLINE_EXCEEDED"
	case ENotFound:
		return "NOT_FOUND"
	case EAlreadyExists:
		return "ALREADY_EXISTS"
	case EPermissionDenied:
		return "PERMISSION_DENIED"
	case EUnauthenticated:
		return "UNAUTHENTICATED"
	case EResourceExhausted:
		return "RESOURCE_EXHAUSTED"
	case EFailedPrecondition:
		return "FAILED_PRECONDITION"
	case EAborted:
		return "ABORTED"
	case EOutOfRange:
		return "OUT_OF_RANGE"
	case EUnimplemented:
		return "UNIMPLEMENTED"
	case EInternal:
		return "INTERNAL"
	case EUnavailable:
		return "UNAVAILABLE"
	case EDataLoss:
		return "DATA_LOSS"
	default:
		return "UNKNOWN"
	}
}
