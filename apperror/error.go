package apperror

import "fmt"

// Error is the typed error a business-logic implementation returns to
// signal a specific failure kind. Handlers recognize *Error via errors.As
// and map Code to the right wire-level status; any other error type maps
// to a generic 500/Internal, so returning a plain error never breaks a
// handler, it only loses the specific status mapping.
//
// Fields are unexported and sealed against invalid zero-value construction
// via composite literal (e.g. Error{message: "x"} silently forgetting code,
// or a bare Error{} degrading unnoticed to ErrorCode(0)/"UNKNOWN"/500) — New
// and Wrap are the only constructors. Use the Code/Message/Cause accessor
// methods to read an *Error's fields.
//
// Discrimination is code-based by design: there are no sentinel errors.
// Callers switch on Code instead of comparing against package-level
// values, so errors.Is matches only caller-supplied causes in the wrap
// chain, never the *Error itself. The cause chain is visible only via
// Unwrap/errors.Is/errors.As — Error never leaks unexpected internals
// beyond the message and cause the caller supplied.
type Error struct {
	code    ErrorCode
	message string
	cause   error
}

// New returns an *Error with no wrapped cause.
// Discrimination is code-based: compare Code against an ErrorCode value
// instead of matching against a sentinel.
func New(code ErrorCode, message string) *Error {
	return &Error{code: code, message: message}
}

// Wrap returns an *Error that wraps cause; Unwrap returns cause. A nil
// cause is allowed and behaves like New. Discrimination is code-based:
// compare Code against an ErrorCode value instead of matching against
// a sentinel.
func Wrap(code ErrorCode, message string, cause error) *Error {
	return &Error{code: code, message: message, cause: cause}
}

// Code returns e's ErrorCode. Code is safe to call on a nil *Error,
// returning the zero ErrorCode.
func (e *Error) Code() ErrorCode {
	if e == nil {
		return ErrorCode(0)
	}

	return e.code
}

// Message returns e's message. Message is safe to call on a nil *Error,
// returning an empty string.
func (e *Error) Message() string {
	if e == nil {
		return ""
	}

	return e.message
}

// Cause returns e's wrapped underlying error, or nil if none. Cause is
// safe to call on a nil *Error, returning nil.
func (e *Error) Cause() error {
	if e == nil {
		return nil
	}

	return e.cause
}

// Error implements the error interface, formatting as "[CODE] message",
// or "[CODE] message: cause" when a cause is present. Error is safe to
// call on a nil *Error, returning an empty string.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}

	if e.cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.code, e.message, e.cause)
	}

	return fmt.Sprintf("[%s] %s", e.code, e.message)
}

// Unwrap returns e's cause, letting errors.Is and errors.As see through
// an *Error to whatever it wraps. Unwrap is safe to call on a nil *Error,
// returning nil.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.cause
}
