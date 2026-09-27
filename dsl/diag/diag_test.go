package diag

import (
	"errors"
	"strconv"
	"testing"
)

func TestPositionString(t *testing.T) {
	tests := []struct {
		name     string
		pos      Position
		expected string
	}{
		{
			name:     "basic position",
			pos:      Position{File: "main.zen", Line: 10, Col: 5},
			expected: "main.zen:10:5",
		},
		{
			name:     "zero column",
			pos:      Position{File: "test.zen", Line: 1, Col: 0},
			expected: "test.zen:1:0",
		},
		{
			name:     "large numbers",
			pos:      Position{File: "bigfile.zen", Line: 9999, Col: 100},
			expected: "bigfile.zen:9999:100",
		},
		{
			name:     "empty file",
			pos:      Position{File: "", Line: 0, Col: 0},
			expected: ":0:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pos.String()
			if got != tt.expected {
				t.Fatalf("Position.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDiagnosticError(t *testing.T) {
	tests := []struct {
		name     string
		diag     *Diagnostic
		expected string
	}{
		{
			name: "basic error message",
			diag: &Diagnostic{
				Pos:   Position{File: "main.zen", Line: 10, Col: 5},
				Phase: "parse",
				Msg:   "unexpected token",
			},
			expected: "main.zen:10:5: [parse] unexpected token",
		},
		{
			name: "lex phase",
			diag: &Diagnostic{
				Pos:   Position{File: "test.zen", Line: 1, Col: 0},
				Phase: "lex",
				Msg:   "invalid character",
			},
			expected: "test.zen:1:0: [lex] invalid character",
		},
		{
			name: "resolve phase",
			diag: &Diagnostic{
				Pos:   Position{File: "resolve.zen", Line: 5, Col: 20},
				Phase: "resolve",
				Msg:   "undefined variable",
			},
			expected: "resolve.zen:5:20: [resolve] undefined variable",
		},
		{
			name: "backend phase",
			diag: &Diagnostic{
				Pos:   Position{File: "gen.zen", Line: 100, Col: 10},
				Phase: "codegen",
				Msg:   "type mismatch",
			},
			expected: "gen.zen:100:10: [codegen] type mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.diag.Error()
			if got != tt.expected {
				t.Fatalf("Diagnostic.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "creates diagnostic without wrapped error",
			run: func(t *testing.T) {
				t.Helper()
				diag := New("parse", Position{File: "main.zen", Line: 10, Col: 5}, "unexpected token")
				if diag == nil {
					t.Fatalf("New() returned nil")
				}

				if diag.Phase != "parse" {
					t.Fatalf("Phase = %q, want %q", diag.Phase, "parse")
				}

				if diag.Msg != "unexpected token" {
					t.Fatalf("Msg = %q, want %q", diag.Msg, "unexpected token")
				}

				if diag.Pos.File != "main.zen" || diag.Pos.Line != 10 || diag.Pos.Col != 5 {
					t.Fatalf("Position mismatch")
				}

				if diag.Wrapped != nil {
					t.Fatalf("Wrapped should be nil, got %v", diag.Wrapped)
				}
			},
		},
		{
			name: "formats message with args",
			run: func(t *testing.T) {
				t.Helper()
				diag := New("lex", Position{File: "test.zen", Line: 1, Col: 0}, "expected %q but got %q", ")", "}}")
				if diag == nil {
					t.Fatalf("New() returned nil")
				}

				expected := `expected ")" but got "}}"`
				if diag.Msg != expected {
					t.Fatalf("Msg = %q, want %q", diag.Msg, expected)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "wraps error as cause",
			run: func(t *testing.T) {
				t.Helper()
				cause := errors.New("original error")

				diag := Wrap("resolve", Position{File: "main.zen", Line: 10, Col: 5}, cause, "failed to resolve")
				if diag == nil {
					t.Fatalf("Wrap() returned nil")
				}

				if !errors.Is(diag.Wrapped, cause) {
					t.Fatalf("Wrapped = %v, want %v", diag.Wrapped, cause)
				}

				if diag.Phase != "resolve" {
					t.Fatalf("Phase = %q, want %q", diag.Phase, "resolve")
				}

				if diag.Msg != "failed to resolve" {
					t.Fatalf("Msg = %q, want %q", diag.Msg, "failed to resolve")
				}
			},
		},
		{
			name: "wraps with formatted message",
			run: func(t *testing.T) {
				t.Helper()
				cause := errors.New("parse failed")

				diag := Wrap("parse", Position{File: "test.zen", Line: 5, Col: 10}, cause, "error at %s", "token")
				if diag.Msg != "error at token" {
					t.Fatalf("Msg = %q, want formatted message", diag.Msg)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestDiagnosticUnwrap(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "Unwrap returns nil for New()",
			run: func(t *testing.T) {
				t.Helper()
				diag := New("parse", Position{File: "main.zen", Line: 1, Col: 1}, "error")
				if diag.Unwrap() != nil {
					t.Fatalf("Unwrap() should return nil, got %v", diag.Unwrap())
				}
			},
		},
		{
			name: "Unwrap returns wrapped error",
			run: func(t *testing.T) {
				t.Helper()
				cause := errors.New("original")

				diag := Wrap("parse", Position{File: "main.zen", Line: 1, Col: 1}, cause, "wrapped")
				if !errors.Is(diag.Unwrap(), cause) {
					t.Fatalf("Unwrap() = %v, want %v", diag.Unwrap(), cause)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestErrorsIsAs(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "errors.Is finds wrapped cause",
			run: func(t *testing.T) {
				t.Helper()
				cause := errors.New("not found")

				diag := Wrap("resolve", Position{File: "main.zen", Line: 1, Col: 1}, cause, "symbol not found")
				if !errors.Is(diag, cause) {
					t.Fatalf("errors.Is(diag, cause) = false, want true")
				}
			},
		},
		{
			name: "errors.As reaches through Unwrap() to find wrapped type",
			run: func(t *testing.T) {
				t.Helper()
				// Use strconv.NumError as a distinct stdlib error type to wrap
				wrappedErr := &strconv.NumError{Func: "ParseInt", Num: "abc", Err: strconv.ErrSyntax}
				diag := Wrap("resolve", Position{File: "main.zen", Line: 1, Col: 1}, wrappedErr, "invalid number")

				// errors.As should reach through Diagnostic.Unwrap() and find the wrapped type
				var found *strconv.NumError
				if !errors.As(diag, &found) {
					t.Fatalf("errors.As(diag, &found) = false, want true (should reach through Unwrap)")
				}

				if found != wrappedErr {
					t.Fatalf("extracted wrapped error mismatch: got %v, want %v", found, wrappedErr)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestListError(t *testing.T) {
	tests := []struct {
		name     string
		list     List
		expected string
	}{
		{
			name: "single diagnostic",
			list: List{
				&Diagnostic{
					Pos:      Position{File: "main.zen", Line: 1, Col: 1},
					Phase:    "lex",
					Msg:      "invalid char",
					Severity: SeverityError,
				},
			},
			expected: "main.zen:1:1: [lex] invalid char",
		},
		{
			name: "multiple diagnostics",
			list: List{
				&Diagnostic{
					Pos:      Position{File: "main.zen", Line: 1, Col: 1},
					Phase:    "lex",
					Msg:      "invalid char",
					Severity: SeverityError,
				},
				&Diagnostic{
					Pos:      Position{File: "main.zen", Line: 2, Col: 5},
					Phase:    "parse",
					Msg:      "unexpected token",
					Severity: SeverityError,
				},
			},
			expected: "main.zen:1:1: [lex] invalid char\nmain.zen:2:5: [parse] unexpected token",
		},
		{
			name:     "empty list",
			list:     List{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.list.Error()
			if got != tt.expected {
				t.Fatalf("List.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestListUnwrap(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "returns slice of wrapped errors",
			run: func(t *testing.T) {
				t.Helper()
				cause1 := errors.New("error 1")
				cause2 := errors.New("error 2")
				list := List{
					Wrap("lex", Position{File: "f1", Line: 1, Col: 1}, cause1, "msg1"),
					Wrap("parse", Position{File: "f2", Line: 2, Col: 2}, cause2, "msg2"),
					New("resolve", Position{File: "f3", Line: 3, Col: 3}, "msg3"),
				}

				errs := list.Unwrap()
				if len(errs) != 3 {
					t.Fatalf("Unwrap() len = %d, want 3", len(errs))
				}

				if !errors.Is(errs[0], cause1) {
					t.Fatalf("errs[0] = %v, want %v", errs[0], cause1)
				}

				if !errors.Is(errs[1], cause2) {
					t.Fatalf("errs[1] = %v, want %v", errs[1], cause2)
				}

				if errs[2] != nil {
					t.Fatalf("errs[2] = %v, want nil", errs[2])
				}
			},
		},
		{
			name: "errors.Is can find cause in list",
			run: func(t *testing.T) {
				t.Helper()
				cause := errors.New("specific error")
				list := List{
					New("lex", Position{File: "f1", Line: 1, Col: 1}, "msg1"),
					Wrap("parse", Position{File: "f2", Line: 2, Col: 2}, cause, "msg2"),
					New("resolve", Position{File: "f3", Line: 3, Col: 3}, "msg3"),
				}

				if !errors.Is(list, cause) {
					t.Fatalf("errors.Is(list, cause) = false, want true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestListHasErrors(t *testing.T) {
	tests := []struct {
		name     string
		list     List
		expected bool
	}{
		{
			name:     "empty list has no errors",
			list:     List{},
			expected: false,
		},
		{
			name: "warning-only list has no errors",
			list: List{
				&Diagnostic{
					Pos:      Position{File: "f1", Line: 1, Col: 1},
					Phase:    "lex",
					Msg:      "warning",
					Severity: SeverityWarning,
				},
				&Diagnostic{
					Pos:      Position{File: "f2", Line: 2, Col: 2},
					Phase:    "parse",
					Msg:      "warning",
					Severity: SeverityWarning,
				},
			},
			expected: false,
		},
		{
			name: "error in list",
			list: List{
				&Diagnostic{
					Pos:      Position{File: "f1", Line: 1, Col: 1},
					Phase:    "lex",
					Msg:      "warning",
					Severity: SeverityWarning,
				},
				&Diagnostic{
					Pos:      Position{File: "f2", Line: 2, Col: 2},
					Phase:    "parse",
					Msg:      "error",
					Severity: SeverityError,
				},
			},
			expected: true,
		},
		{
			name: "all errors",
			list: List{
				&Diagnostic{
					Pos:      Position{File: "f1", Line: 1, Col: 1},
					Phase:    "lex",
					Msg:      "error1",
					Severity: SeverityError,
				},
				&Diagnostic{
					Pos:      Position{File: "f2", Line: 2, Col: 2},
					Phase:    "parse",
					Msg:      "error2",
					Severity: SeverityError,
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.list.HasErrors()
			if got != tt.expected {
				t.Fatalf("List.HasErrors() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestListSorted(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "sorts by file, then line, then col",
			run: func(t *testing.T) {
				t.Helper()
				list := List{
					&Diagnostic{Pos: Position{File: "b.zen", Line: 1, Col: 1}, Phase: "lex", Msg: "b1"},
					&Diagnostic{Pos: Position{File: "a.zen", Line: 2, Col: 1}, Phase: "lex", Msg: "a2"},
					&Diagnostic{Pos: Position{File: "a.zen", Line: 1, Col: 2}, Phase: "lex", Msg: "a12"},
					&Diagnostic{Pos: Position{File: "a.zen", Line: 1, Col: 1}, Phase: "lex", Msg: "a11"},
					&Diagnostic{Pos: Position{File: "c.zen", Line: 1, Col: 1}, Phase: "lex", Msg: "c1"},
				}

				sorted := list.Sorted()

				expected := []string{"a11", "a12", "a2", "b1", "c1"}
				if len(sorted) != len(expected) {
					t.Fatalf("sorted len = %d, want %d", len(sorted), len(expected))
				}

				for i, exp := range expected {
					if sorted[i].Msg != exp {
						t.Fatalf("sorted[%d].Msg = %q, want %q", i, sorted[i].Msg, exp)
					}
				}
			},
		},
		{
			name: "stable sort for equal positions",
			run: func(t *testing.T) {
				t.Helper()
				list := List{
					&Diagnostic{Pos: Position{File: "a.zen", Line: 1, Col: 1}, Phase: "lex", Msg: "first"},
					&Diagnostic{Pos: Position{File: "a.zen", Line: 1, Col: 1}, Phase: "parse", Msg: "second"},
					&Diagnostic{Pos: Position{File: "a.zen", Line: 1, Col: 1}, Phase: "resolve", Msg: "third"},
				}

				sorted := list.Sorted()

				if sorted[0].Msg != "first" || sorted[1].Msg != "second" || sorted[2].Msg != "third" {
					t.Fatalf("stable sort failed: got %v", []string{sorted[0].Msg, sorted[1].Msg, sorted[2].Msg})
				}
			},
		},
		{
			name: "empty list",
			run: func(t *testing.T) {
				t.Helper()
				list := List{}

				sorted := list.Sorted()
				if len(sorted) != 0 {
					t.Fatalf("sorted empty list len = %d, want 0", len(sorted))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
