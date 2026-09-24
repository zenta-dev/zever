package permission

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// coverStubChecker is a distinct stub for cover tests.
// It avoids clashing with stubChecker in permission_test.go.
type coverStubChecker struct{}

// Can always allows with reason allow.
func (coverStubChecker) Can(_ context.Context, _ Subject, _ string, _ Resource) (Decision, error) {
	return Decision{Allowed: true, Reason: "allow"}, nil
}

func TestCover_Errors_ExactStrings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"duplicate rbac", DuplicateError{Adapter: RBAC}, "permission: duplicate registration: rbac"},
		{"unknown rbac", UnknownAdapterError{Adapter: RBAC}, "permission: unknown adapter: rbac (forgotten import?)"},
		{"unknown unregistered", UnknownAdapterError{Adapter: Adapter(9401)}, "permission: unknown adapter: unknown (forgotten import?)"},
		{"invalid x", InvalidAdapterError{Adapter: "x"}, "permission: invalid adapter: \"x\""},
		{"invalid options", InvalidOptionsError{Reason: "boom"}, "permission: invalid options: boom"},
	}
	for _, tc := range cases {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("%s: Error() = %q want %q", tc.name, got, tc.want)
		}
	}
	if !errors.Is(DuplicateError{Adapter: RBAC}, ErrDuplicate) {
		t.Error("DuplicateError does not match ErrDuplicate")
	}
	if !errors.Is(UnknownAdapterError{Adapter: RBAC}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError does not match ErrUnknownAdapter")
	}
	if !errors.Is(InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError does not match ErrInvalidAdapter")
	}
	if !errors.Is(InvalidOptionsError{Reason: "boom"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError does not match ErrInvalidOptions")
	}
}

func TestCover_Options_Validate_Gaps(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 257)
	cases := []struct {
		name   string
		opts   Options
		reason string
	}{
		{"rule action overlong", Options{Rules: []Rule{{Role: "admin", Action: long}}}, "rule action must be at most 256 characters"},
		{"model path dotdot", Options{ModelPath: "../m.conf", PolicyPath: "p.csv"}, "model_path must not contain .."},
		{"policy path dotdot", Options{ModelPath: "m.conf", PolicyPath: "../p.csv"}, "policy_path must not contain .."},
		{"role value overlong", Options{Roles: map[string][]string{"admin": {long}}}, "role_value must be at most 256 characters"},
	}
	for _, tc := range cases {
		err := tc.opts.Validate()
		if err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
			continue
		}
		if !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("%s: err %v does not match ErrInvalidOptions", tc.name, err)
		}
		var ioe *InvalidOptionsError
		if !errors.As(err, &ioe) {
			t.Errorf("%s: err %T is not *InvalidOptionsError", tc.name, err)
			continue
		}
		if ioe.Reason != tc.reason {
			t.Errorf("%s: reason = %q want %q", tc.name, ioe.Reason, tc.reason)
		}
	}
}

func TestCover_Options_Validate_PolicyMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	model := filepath.Join(dir, "model.conf")
	if err := os.WriteFile(model, []byte("m"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.csv")
	err := (Options{ModelPath: model, PolicyPath: missing}).Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("policy-missing err = %v, want ErrInvalidOptions", err)
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
	if ioe.Reason != "policy_path does not exist" {
		t.Fatalf("reason = %q want %q", ioe.Reason, "policy_path does not exist")
	}
}

func TestCover_Open_Success9201(t *testing.T) {
	a := Adapter(9201)
	if err := Register(a, func(Options) (Checker, error) { return coverStubChecker{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	c, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if c == nil {
		t.Fatal("Open checker is nil")
	}
	d, err := c.Can(t.Context(), Subject{}, "read", Resource{})
	if err != nil {
		t.Fatalf("Can err = %v", err)
	}
	if !d.Allowed || d.Reason != "allow" {
		t.Fatalf("decision = %+v, want {Allowed:true Reason:allow}", d)
	}
}

func TestCover_Open_FactoryError9202(t *testing.T) {
	a := Adapter(9202)
	sentinel := errors.New("cover boom 9202")
	if err := Register(a, func(Options) (Checker, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "permission: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "permission: open")
	}
}
