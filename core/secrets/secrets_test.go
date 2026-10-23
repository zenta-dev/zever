package secrets_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

type stubSecrets struct{}

func (stubSecrets) Get(context.Context, string) ([]byte, error) { return []byte("v"), nil }
func (stubSecrets) Set(context.Context, string, []byte) error   { return nil }
func (stubSecrets) Delete(context.Context, string) error        { return nil }
func (stubSecrets) List(context.Context) ([]string, error)      { return []string{"a"}, nil }
func (stubSecrets) Close(context.Context) error                 { return nil }

var _ secrets.Secrets = stubSecrets{}

func TestValidateName_table(t *testing.T) {
	t.Parallel()

	valid := []string{"my-secret", "db.password", "api_key_2024", "a", "SECRET-123"}
	for _, name := range valid {
		if err := secrets.ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) error = %v", name, err)
		}
	}

	invalid := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "slash", input: "a/b"},
		{name: "dotdot", input: "a..b"},
		{name: "traversal", input: "../etc/passwd"},
		{name: "spaces", input: "name with spaces"},
		{name: "special", input: "key@special"},
		{name: "semicolon", input: "key;DROP"},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := secrets.ValidateName(tt.input); !errors.Is(err, secrets.ErrInvalidKey) {
				t.Fatalf("ValidateName(%q) = %v, want ErrInvalidKey", tt.input, err)
			}
		})
	}
}
