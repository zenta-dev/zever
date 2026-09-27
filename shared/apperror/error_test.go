package apperror

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestNew_noCause(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    ErrorCode
		message string
	}{
		{"NotFound", NotFound, "no such task"},
		{"Internal", Internal, "something broke"},
		{"InvalidArgument", InvalidArgument, "bad input"},
		{"EmptyMessage", NotFound, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := New(tt.code, tt.message)

			if err.Code() != tt.code {
				t.Fatalf("Code() = %v, want %v", err.Code(), tt.code)
			}

			if err.Message() != tt.message {
				t.Fatalf("Message() = %q, want %q", err.Message(), tt.message)
			}

			if err.Cause() != nil {
				t.Fatalf("Cause() = %v, want nil", err.Cause())
			}

			if err.Unwrap() != nil {
				t.Fatalf("Unwrap() = %v, want nil", err.Unwrap())
			}
		})
	}
}

func TestWrap_nilCauseBehavesLikeNew(t *testing.T) {
	t.Parallel()

	err := Wrap(NotFound, "missing", nil)

	if err.Code() != NotFound {
		t.Fatalf("Code() = %v, want %v", err.Code(), NotFound)
	}

	if err.Message() != "missing" {
		t.Fatalf("Message() = %q, want %q", err.Message(), "missing")
	}

	if err.Cause() != nil {
		t.Fatalf("Cause() = %v, want nil", err.Cause())
	}

	if err.Unwrap() != nil {
		t.Fatalf("Unwrap() = %v, want nil", err.Unwrap())
	}

	if got, want := err.Error(), "[NOT_FOUND] missing"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestWrap_unwrapChain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    ErrorCode
		message string
		cause   error
	}{
		{"PlainCause", Internal, "something broke", errors.New("boom")},
		{"EmptyMessageWithCause", Internal, "", errors.New("root")},
		{"WrappedCause", NotFound, "missing", fmt.Errorf("lookup: %w", errors.New("db down"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Wrap(tt.code, tt.message, tt.cause)

			if !errors.Is(err, tt.cause) {
				t.Fatalf("errors.Is(err, cause) = false, want true")
			}

			var target *Error
			if !errors.As(err, &target) {
				t.Fatalf("errors.As(err, &target) = false, want true")
			}

			if target.Code() != tt.code {
				t.Fatalf("Code() = %v, want %v", target.Code(), tt.code)
			}

			if target.Message() != tt.message {
				t.Fatalf("Message() = %q, want %q", target.Message(), tt.message)
			}

			if tt.cause == nil {
				if target.Cause() != nil {
					t.Fatalf("Cause() = %v, want nil", target.Cause())
				}
			} else if !errors.Is(target.Cause(), tt.cause) {
				t.Fatalf("Cause() = %v, want %v", target.Cause(), tt.cause)
			}
		})
	}
}

func TestWrap_multiLevelAsResolvesThroughFmtWrapping(t *testing.T) {
	t.Parallel()

	root := errors.New("root cause")
	inner := Wrap(NotFound, "missing", root)
	outer := fmt.Errorf("handler failed: %w", inner)

	var target *Error
	if !errors.As(outer, &target) {
		t.Fatalf("errors.As(outer, &target) = false, want true")
	}

	if target.Code() != NotFound {
		t.Fatalf("Code() = %v, want %v", target.Code(), NotFound)
	}

	if !errors.Is(outer, root) {
		t.Fatalf("errors.Is(outer, root) = false, want true")
	}

	if !errors.Is(outer, inner) {
		t.Fatalf("errors.Is(outer, inner) = false, want true")
	}
}

func TestWrap_nestedApperrorChain(t *testing.T) {
	t.Parallel()

	root := errors.New("disk gone")
	mid := Wrap(DataLoss, "write failed", root)
	outer := Wrap(Internal, "request failed", mid)

	var target *Error
	if !errors.As(outer, &target) {
		t.Fatalf("errors.As(outer, &target) = false, want true")
	}

	if target != outer {
		t.Fatalf("As resolved to %v, want outermost *Error", target)
	}

	if !errors.Is(outer, mid) {
		t.Fatalf("errors.Is(outer, mid) = false, want true")
	}

	if !errors.Is(outer, root) {
		t.Fatalf("errors.Is(outer, root) = false, want true")
	}
}

func TestError_exactFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{"NoCause", New(InvalidArgument, "bad input"), "[INVALID_ARGUMENT] bad input"},
		{"WithCause", Wrap(Internal, "wrapper", errors.New("root cause")), "[INTERNAL] wrapper: root cause"},
		{"EmptyMessage", New(NotFound, ""), "[NOT_FOUND] "},
		{"EmptyMessageWithCause", Wrap(Internal, "", errors.New("root")), "[INTERNAL] : root"},
		{"NilCauseBehavesLikeNew", Wrap(NotFound, "missing", nil), "[NOT_FOUND] missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestError_codePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code ErrorCode
		want string
	}{
		{"NotFound", NotFound, "[NOT_FOUND]"},
		{"Internal", Internal, "[INTERNAL]"},
		{"Unauthenticated", Unauthenticated, "[UNAUTHENTICATED]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := New(tt.code, "msg")
			if !strings.HasPrefix(err.Error(), tt.want) {
				t.Fatalf("Error() = %q, want prefix %q", err.Error(), tt.want)
			}

			wrapped := Wrap(tt.code, "msg", errors.New("cause"))
			if !strings.HasPrefix(wrapped.Error(), tt.want) {
				t.Fatalf("Error() = %q, want prefix %q", wrapped.Error(), tt.want)
			}
		})
	}
}

func TestAccessors_exposeConstructedFields(t *testing.T) {
	t.Parallel()

	cause := errors.New("boom")
	err := Wrap(Internal, "something broke", cause)

	if err.Code() != Internal {
		t.Fatalf("Code() = %v, want %v", err.Code(), Internal)
	}

	if err.Message() != "something broke" {
		t.Fatalf("Message() = %q, want %q", err.Message(), "something broke")
	}

	if !errors.Is(err.Cause(), cause) {
		t.Fatalf("Cause() = %v, want %v", err.Cause(), cause)
	}
}

func TestNilError_allMethodsSafe(t *testing.T) {
	t.Parallel()

	var err *Error

	if err.Code() != ErrorCode(0) {
		t.Fatalf("nil.Code() = %v, want zero value", err.Code())
	}

	if err.Message() != "" {
		t.Fatalf("nil.Message() = %q, want empty string", err.Message())
	}

	if err.Cause() != nil {
		t.Fatalf("nil.Cause() = %v, want nil", err.Cause())
	}

	if err.Error() != "" {
		t.Fatalf("nil.Error() = %q, want empty string", err.Error())
	}

	if err.Unwrap() != nil {
		t.Fatalf("nil.Unwrap() = %v, want nil", err.Unwrap())
	}
}

func TestNilError_typedNilMatchesAsButStaysSafe(t *testing.T) {
	t.Parallel()

	var err *Error

	var target *Error
	// A typed-nil *Error in a non-nil error interface still matches
	// errors.As; the target is set to nil and stays safe to use.
	if !errors.As(err, &target) {
		t.Fatalf("errors.As(nil *Error, &target) = false, want true")
	}

	if target != nil {
		t.Fatalf("As target = %v, want nil", target)
	}

	if target.Code() != ErrorCode(0) {
		t.Fatalf("target.Code() = %v, want zero value", target.Code())
	}

	if target.Error() != "" {
		t.Fatalf("target.Error() = %q, want empty string", target.Error())
	}
}

func TestZeroValueError_degradesSanely(t *testing.T) {
	t.Parallel()

	var zero Error

	if got := zero.Code().String(); got != "UNKNOWN" {
		t.Fatalf("zero-value Code().String() = %q, want %q", got, "UNKNOWN")
	}

	if got := zero.Code().HTTPStatus(); got != 500 {
		t.Fatalf("zero-value Code().HTTPStatus() = %d, want 500", got)
	}

	if got := zero.Message(); got != "" {
		t.Fatalf("zero-value Message() = %q, want empty string", got)
	}

	if zero.Cause() != nil {
		t.Fatalf("zero-value Cause() = %v, want nil", zero.Cause())
	}

	if zero.Unwrap() != nil {
		t.Fatalf("zero-value Unwrap() = %v, want nil", zero.Unwrap())
	}

	if got, want := zero.Error(), "[UNKNOWN] "; got != want {
		t.Fatalf("zero-value Error() = %q, want %q", got, want)
	}
}

func TestTransportHandoff_asThroughWrappedChain(t *testing.T) {
	t.Parallel()

	// Simulates a transport boundary: the handler receives an error
	// wrapped by intermediate layers and must recover the *Error to
	// map its Code to a wire status.
	root := Wrap(Unavailable, "upstream down", errors.New("conn refused"))
	transport := fmt.Errorf("rpc failed: %w", fmt.Errorf("retry exhausted: %w", root))

	var target *Error
	if !errors.As(transport, &target) {
		t.Fatalf("errors.As(transport, &target) = false, want true")
	}

	if target.Code() != Unavailable {
		t.Fatalf("Code() = %v, want %v", target.Code(), Unavailable)
	}

	if got := target.Code().HTTPStatus(); got != 503 {
		t.Fatalf("HTTPStatus() = %d, want 503", got)
	}
}

func TestConcurrent_newWrapAndErrorRaceSafe(t *testing.T) {
	t.Parallel()

	codes := []ErrorCode{NotFound, Internal, InvalidArgument, Unavailable, Unknown}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code := codes[i%len(codes)]
			_ = New(code, "concurrent").Error()
			_ = Wrap(code, "concurrent", errors.New("cause")).Error()
			_ = Wrap(code, "concurrent", nil).Code()
		}(i)
	}
	wg.Wait()
}
