package env

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zenta-dev/zever/secrets"
)

// adapter reads secrets from environment variables sharing a prefix.
// It is read-only: Set and Delete report secrets.ErrNotSupported.
type adapter struct {
	prefix string
}

// New creates an env secrets adapter from validated Options.
func New(opts Options) (secrets.Secrets, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	return &adapter{prefix: opts.Prefix}, nil
}

// Get returns the value of the prefixed environment variable for name.
// The value is never included in returned errors.
func (a *adapter) Get(_ context.Context, name string) ([]byte, error) {
	if err := secrets.ValidateName(name); err != nil {
		return nil, err
	}

	val, ok := os.LookupEnv(a.boundary() + name)
	if !ok {
		return nil, fmt.Errorf("env: %w: %s", secrets.ErrNotFound, name)
	}

	return []byte(val), nil
}

// boundary returns the prefix with a trailing underscore separator.
func (a *adapter) boundary() string {
	if a.prefix == "" {
		return ""
	}

	if strings.HasSuffix(a.prefix, "_") {
		return a.prefix
	}

	return a.prefix + "_"
}

// Set always fails: environment-backed secrets are read-only.
func (a *adapter) Set(_ context.Context, _ string, _ []byte) error {
	return fmt.Errorf("env: %w", secrets.ErrNotSupported)
}

// Delete always fails: environment-backed secrets are read-only.
func (a *adapter) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("env: %w", secrets.ErrNotSupported)
}

// List returns secret names stripped of the prefix boundary.
// Entries not under the boundary are excluded.
func (a *adapter) List(_ context.Context) ([]string, error) {
	boundary := a.boundary()
	envs := os.Environ()

	var keys []string

	for _, e := range envs {
		k, _, _ := strings.Cut(e, "=")
		if !strings.HasPrefix(k, boundary) || len(k) <= len(boundary) {
			continue
		}

		keys = append(keys, k[len(boundary):])
	}

	return keys, nil
}

// Close releases no resources and always succeeds.
func (a *adapter) Close(_ context.Context) error {
	return nil
}
