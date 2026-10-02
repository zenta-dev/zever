package permission

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptions_Validate_valid_minimal(t *testing.T) {
	t.Parallel()
	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("empty options err = %v, want nil (deny-all)", err)
	}
	opts := Options{
		Rules: []Rule{{Role: "admin", Action: "read"}},
		Roles: map[string][]string{"admin": {"user"}},
	}
	if err := opts.Validate(); err != nil {
		t.Fatalf("valid options err = %v", err)
	}
}

func TestOptions_Validate_invalid_rules(t *testing.T) {
	t.Parallel()
	opts := Options{Rules: []Rule{{Action: "read"}}}
	if err := opts.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("invalid rule err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_model_without_policy(t *testing.T) {
	t.Parallel()
	if err := (Options{ModelPath: "m.conf"}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("model-only err = %v, want ErrInvalidOptions", err)
	}
	if err := (Options{PolicyPath: "p.csv"}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("policy-only err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_nonexistent_path(t *testing.T) {
	t.Parallel()
	opts := Options{ModelPath: "no-such-model.conf", PolicyPath: "no-such-policy.csv"}
	if err := opts.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("nonexistent err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_path_escape(t *testing.T) {
	t.Parallel()
	opts := Options{ModelPath: "../m.conf", PolicyPath: "../p.csv"}
	if err := opts.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("escape err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_valid_paths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := filepath.Join(dir, "model.conf")
	p := filepath.Join(dir, "policy.csv")
	if err := os.WriteFile(m, []byte("m"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("p"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (Options{ModelPath: m, PolicyPath: p}).Validate(); err != nil {
		t.Fatalf("valid paths err = %v", err)
	}
}

func TestOptions_Validate_roles_empty_key(t *testing.T) {
	t.Parallel()
	opts := Options{Roles: map[string][]string{"": {"user"}}}
	if err := opts.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("empty key err = %v, want ErrInvalidOptions", err)
	}
	opts = Options{Roles: map[string][]string{"admin": {""}}}
	if err := opts.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("empty value err = %v, want ErrInvalidOptions", err)
	}
	opts = Options{Roles: map[string][]string{strings.Repeat("x", 257): {"user"}}}
	if err := opts.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("overlong key err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_rulesWithModelRejected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	m := filepath.Join(dir, "model.conf")
	p := filepath.Join(dir, "policy.csv")
	if err := os.WriteFile(m, []byte("m"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("p"), 0o600); err != nil {
		t.Fatal(err)
	}

	rules := []Rule{{Role: "admin", Action: "read"}}
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{name: "rules with model rejected", opts: Options{Rules: rules, ModelPath: m, PolicyPath: p}, wantErr: true},
		{name: "roles with model allowed", opts: Options{Roles: map[string][]string{"u": {"admin"}}, ModelPath: m, PolicyPath: p}, wantErr: false},
		{name: "rules without model allowed", opts: Options{Rules: rules}, wantErr: false},
		{name: "model without rules allowed", opts: Options{ModelPath: m, PolicyPath: p}, wantErr: false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.opts.Validate()
			if tc.wantErr && !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Validate() err = %v, want ErrInvalidOptions", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() err = %v, want nil", err)
			}
		})
	}
}
