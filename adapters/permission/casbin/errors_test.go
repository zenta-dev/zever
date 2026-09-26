package casbin

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/permission"
)

func TestSentinelMessages(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrNilEnforcer, "casbin: nil enforcer"},
		{ErrDuplicatePolicy, "casbin: duplicate policy"},
	}
	for _, tc := range cases {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

func TestDuplicatePolicyIsAs(t *testing.T) {
	opts := permission.Options{Rules: []permission.Rule{
		{Role: "admin", Action: "read"},
		{Role: "admin", Action: "read"},
	}}
	_, err := New(opts)
	if err == nil {
		t.Fatal("New = nil error, want duplicate policy error")
	}
	if !errors.Is(err, ErrDuplicatePolicy) {
		t.Errorf("errors.Is(%q, ErrDuplicatePolicy) = false, want true", err.Error())
	}
	var dpe *DuplicatePolicyError
	if !errors.As(err, &dpe) {
		t.Fatalf("errors.As(%q) = false, want DuplicatePolicyError", err.Error())
	}
	if dpe.Role != "admin" {
		t.Errorf("Role = %q, want %q", dpe.Role, "admin")
	}
	if dpe.Action != "read" {
		t.Errorf("Action = %q, want %q", dpe.Action, "read")
	}
}

func TestNilEnforcerIs(t *testing.T) {
	_, err := newChecker(nil, nil)
	if err == nil {
		t.Fatal("newChecker(nil) = nil error, want ErrNilEnforcer")
	}
	if !errors.Is(err, ErrNilEnforcer) {
		t.Errorf("errors.Is(%q, ErrNilEnforcer) = false, want true", err.Error())
	}
}
