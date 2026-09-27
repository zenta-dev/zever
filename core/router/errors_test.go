package router

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinels_prefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
	}{
		{name: "nil_factory", err: ErrNilFactory},
		{name: "duplicate", err: ErrDuplicateAdapter},
		{name: "unknown_adapter", err: ErrUnknownAdapter},
		{name: "invalid_adapter", err: ErrInvalidAdapter},
		{name: "invalid_options", err: ErrInvalidOptions},
		{name: "invalid_method", err: ErrInvalidMethod},
		{name: "malformed_pattern", err: ErrMalformedPattern},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err == nil {
				t.Fatal("sentinel is nil")
			}
			if !strings.HasPrefix(tc.err.Error(), "router: ") {
				t.Fatalf("sentinel %q missing prefix %q", tc.err.Error(), "router: ")
			}
		})
	}
}

func TestErrors_unwrap_carries_sentinel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "duplicate", err: &DuplicateAdapterError{Adapter: AdapterFiber}, want: ErrDuplicateAdapter},
		{name: "unknown", err: &UnknownAdapterError{Adapter: AdapterFiber}, want: ErrUnknownAdapter},
		{name: "invalid_adapter", err: &InvalidAdapterError{Adapter: "x"}, want: ErrInvalidAdapter},
		{name: "invalid_options", err: &InvalidOptionsError{Reason: "x"}, want: ErrInvalidOptions},
		{name: "invalid_method", err: &InvalidMethodError{Method: "x"}, want: ErrInvalidMethod},
		{name: "malformed", err: &MalformedPatternError{Pattern: "x", Reason: "y"}, want: ErrMalformedPattern},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !errors.Is(tc.err, tc.want) {
				t.Fatalf("errors.Is(%T, %v) = false, want true", tc.err, tc.want)
			}
		})
	}
}

func TestErrors_duplicate_aliases_compat(t *testing.T) {
	t.Parallel()
	e := &DuplicateAdapterError{Adapter: AdapterFiber}
	if !errors.Is(e, ErrDuplicate) {
		t.Fatal("Is ErrDuplicate alias = false")
	}
	var de *DuplicateError
	if !errors.As(e, &de) {
		t.Fatalf("err type = %T, want *DuplicateError alias", e)
	}
	if !errors.Is(ErrDuplicateAdapter, ErrDuplicate) {
		t.Errorf("alias ErrDuplicate does not match ErrDuplicateAdapter")
	}
}

func TestErrors_typed_As(t *testing.T) {
	t.Parallel()
	t.Run("invalid_adapter", func(t *testing.T) {
		t.Parallel()
		_, err := ParseAdapter("")
		var iae *InvalidAdapterError
		if !errors.As(err, &iae) {
			t.Fatalf("err type = %T, want *InvalidAdapterError", err)
		}
		if iae.Adapter != "" {
			t.Fatalf("Adapter = %q, want %q", iae.Adapter, "")
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()
		e := &DuplicateAdapterError{Adapter: AdapterFiber}
		var de *DuplicateAdapterError
		if !errors.As(e, &de) {
			t.Fatalf("err type = %T, want *DuplicateAdapterError", e)
		}
		if de.Adapter != AdapterFiber {
			t.Fatalf("Adapter = %v, want %v", de.Adapter, AdapterFiber)
		}
		if !strings.Contains(de.Error(), "fiber") {
			t.Fatalf("Error() = %q, want adapter name", de.Error())
		}
		if !errors.Is(de, ErrDuplicateAdapter) {
			t.Fatal("Is ErrDuplicateAdapter = false")
		}
	})
	t.Run("unknown", func(t *testing.T) {
		t.Parallel()
		e := &UnknownAdapterError{Adapter: Adapter("")}
		var ue *UnknownAdapterError
		if !errors.As(e, &ue) {
			t.Fatalf("err type = %T, want *UnknownAdapterError", e)
		}
		if ue.Adapter != Adapter("") {
			t.Fatalf("Adapter = %v, want 99", ue.Adapter)
		}
		if !strings.Contains(ue.Error(), "unknown") {
			t.Fatalf("Error() = %q, want unknown", ue.Error())
		}
	})
	t.Run("invalid_options", func(t *testing.T) {
		t.Parallel()
		e := &InvalidOptionsError{Reason: "app_name must be at most 64 characters"}
		var ioe *InvalidOptionsError
		if !errors.As(e, &ioe) {
			t.Fatalf("err type = %T, want *InvalidOptionsError", e)
		}
		if ioe.Reason != "app_name must be at most 64 characters" {
			t.Fatalf("Reason = %q", ioe.Reason)
		}
		if !strings.Contains(ioe.Error(), "at most 64") {
			t.Fatalf("Error() = %q, want reason", ioe.Error())
		}
		if !errors.Is(ioe, ErrInvalidOptions) {
			t.Fatal("Is ErrInvalidOptions = false")
		}
	})
	t.Run("invalid_method", func(t *testing.T) {
		t.Parallel()
		e := &InvalidMethodError{Method: "FETCH"}
		var ime *InvalidMethodError
		if !errors.As(e, &ime) {
			t.Fatalf("err type = %T, want *InvalidMethodError", e)
		}
		if ime.Method != "FETCH" {
			t.Fatalf("Method = %q, want %q", ime.Method, "FETCH")
		}
		if !strings.Contains(ime.Error(), "FETCH") {
			t.Fatalf("Error() = %q, want method", ime.Error())
		}
		if !errors.Is(ime, ErrInvalidMethod) {
			t.Fatal("Is ErrInvalidMethod = false")
		}
	})
	t.Run("malformed_pattern", func(t *testing.T) {
		t.Parallel()
		e := &MalformedPatternError{Pattern: "/a/{b", Reason: "unclosed brace"}
		var mpe *MalformedPatternError
		if !errors.As(e, &mpe) {
			t.Fatalf("err type = %T, want *MalformedPatternError", e)
		}
		if mpe.Pattern != "/a/{b" {
			t.Fatalf("Pattern = %q", mpe.Pattern)
		}
		if mpe.Reason != "unclosed brace" {
			t.Fatalf("Reason = %q", mpe.Reason)
		}
		if !strings.Contains(mpe.Error(), "/a/{b") {
			t.Fatalf("Error() = %q, want pattern", mpe.Error())
		}
		if !strings.Contains(mpe.Error(), "unclosed brace") {
			t.Fatalf("Error() = %q, want reason", mpe.Error())
		}
		if !errors.Is(mpe, ErrMalformedPattern) {
			t.Fatal("Is ErrMalformedPattern = false")
		}
	})
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	de := &DuplicateAdapterError{Adapter: AdapterFiber}
	if de.Adapter != AdapterFiber {
		t.Fatalf("DuplicateAdapterError.Adapter = %v", de.Adapter)
	}
	ue := &UnknownAdapterError{Adapter: Adapter("test-5")}
	if ue.Adapter != Adapter("test-5") {
		t.Fatalf("UnknownAdapterError.Adapter = %v", ue.Adapter)
	}
	iae := &InvalidAdapterError{Adapter: "nope"}
	if iae.Adapter != "nope" {
		t.Fatalf("InvalidAdapterError.Adapter = %q", iae.Adapter)
	}
	ioe := &InvalidOptionsError{Reason: "bad"}
	if ioe.Reason != "bad" {
		t.Fatalf("InvalidOptionsError.Reason = %q", ioe.Reason)
	}
	ime := &InvalidMethodError{Method: "BAD"}
	if ime.Method != "BAD" {
		t.Fatalf("InvalidMethodError.Method = %q", ime.Method)
	}
	mpe := &MalformedPatternError{Pattern: "p", Reason: "r"}
	if mpe.Pattern != "p" || mpe.Reason != "r" {
		t.Fatalf("MalformedPatternError = %+v", mpe)
	}
}
