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
		var ioe InvalidOptionsError
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
	var ioe InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
	if ioe.Reason != "policy_path does not exist" {
		t.Fatalf("reason = %q want %q", ioe.Reason, "policy_path does not exist")
	}
}

// errCoverOpenFactory is process-shared so repeat runs (-count=2)
// rewire the same sentinel instance; Register tolerates the duplicate.
var errCoverOpenFactory = errors.New("cover boom 9202")
