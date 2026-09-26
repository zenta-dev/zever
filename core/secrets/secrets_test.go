package secrets_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"fmt"

	"github.com/zenta-dev/zever/adapters/secrets/env"
	"github.com/zenta-dev/zever/core/secrets"
)

var secretsAdapterSeq atomic.Int64

func freshSecretsAdapter() secrets.Adapter {
	return secrets.Adapter(fmt.Sprintf("test-%d", 1000+secretsAdapterSeq.Add(1)))
}

type stubSecrets struct{}

func (stubSecrets) Get(context.Context, string) ([]byte, error) { return []byte("v"), nil }
func (stubSecrets) Set(context.Context, string, []byte) error   { return nil }
func (stubSecrets) Delete(context.Context, string) error        { return nil }
func (stubSecrets) List(context.Context) ([]string, error)      { return []string{"a"}, nil }
func (stubSecrets) Close(context.Context) error                 { return nil }

var _ secrets.Secrets = stubSecrets{}

func envFactory(opts secrets.Options) (secrets.Secrets, error) {
	return env.New(env.Options{Prefix: opts.Prefix})
}

func TestSecretsOpen_registeredFactory_returnsSecrets(t *testing.T) {
	a := freshSecretsAdapter()
	wantOpts := secrets.Options{Prefix: "T_"}
	var gotOpts secrets.Options

	if err := secrets.Register(a, func(opts secrets.Options) (secrets.Secrets, error) {
		gotOpts = opts
		return stubSecrets{}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := secrets.Open(a, wantOpts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if got == nil {
		t.Fatal("Open() = nil, want secrets")
	}

	if gotOpts != wantOpts {
		t.Errorf("factory opts = %+v, want %+v", gotOpts, wantOpts)
	}

	val, err := got.Get(t.Context(), "k")
	if err != nil || string(val) != "v" {
		t.Fatalf("Get = (%q, %v), want (v, nil)", val, err)
	}
}

func TestSecretsRegister_nilFactory_returnsNilFactory(t *testing.T) {
	err := secrets.Register(freshSecretsAdapter(), nil)
	if err == nil {
		t.Fatal("Register(nil) = nil, want ErrNilFactory")
	}

	if !errors.Is(err, secrets.ErrNilFactory) {
		t.Errorf("errors.Is(err, ErrNilFactory) = false (err = %v)", err)
	}
}

func TestSecretsRegister_duplicate_returnsDuplicate(t *testing.T) {
	a := freshSecretsAdapter()
	stub := func(secrets.Options) (secrets.Secrets, error) { return stubSecrets{}, nil }

	if err := secrets.Register(a, stub); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err := secrets.Register(a, stub)
	if err == nil {
		t.Fatal("Register(dup) = nil, want ErrDuplicate")
	}

	if !errors.Is(err, secrets.ErrDuplicate) {
		t.Errorf("errors.Is(err, ErrDuplicate) = false (err = %v)", err)
	}

	var dupErr *secrets.DuplicateError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As(err, DuplicateError) = false (err = %T %v)", err, err)
	}

	if dupErr.Adapter != a {
		t.Errorf("DuplicateError.Adapter = %v, want %v", dupErr.Adapter, a)
	}
}

func TestSecretsOpen_unknown_returnsUnknownAdapter(t *testing.T) {
	_, err := secrets.Open(secrets.Adapter("test-9999"), secrets.Options{})
	if err == nil {
		t.Fatal("Open() = nil, want ErrUnknownAdapter")
	}

	if !errors.Is(err, secrets.ErrUnknownAdapter) {
		t.Errorf("errors.Is(err, ErrUnknownAdapter) = false (err = %v)", err)
	}

	var unkErr *secrets.UnknownAdapterError
	if !errors.As(err, &unkErr) {
		t.Fatalf("errors.As(err, UnknownAdapterError) = false (err = %T %v)", err, err)
	}
}

func TestSecretsOpen_factoryError_wrapped(t *testing.T) {
	a := freshSecretsAdapter()
	sentinel := errors.New("boom")
	_ = secrets.Register(a, func(secrets.Options) (secrets.Secrets, error) { return nil, sentinel })

	_, err := secrets.Open(a, secrets.Options{})
	if err == nil {
		t.Fatal("Open() = nil, want wrapped error")
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
	}
}

func TestSecretsOpen_concurrentReads_safe(t *testing.T) {
	a := freshSecretsAdapter()
	_ = secrets.Register(a, func(secrets.Options) (secrets.Secrets, error) { return stubSecrets{}, nil })

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := secrets.Open(a, secrets.Options{}); err != nil {
				t.Errorf("Open() error = %v", err)
			}
		}()
	}

	wg.Wait()
}

func TestSecretsOpen_envAdapter_endToEnd(t *testing.T) {
	a := freshSecretsAdapter()

	if err := secrets.Register(a, envFactory); err != nil {
		t.Fatalf("Register(env) error = %v", err)
	}

	s, err := secrets.Open(a, secrets.Options{Prefix: "E2E_S"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	ctx := t.Context()
	defer func() { _ = s.Close(ctx) }()

	t.Setenv("E2E_S_TOKEN", "abc")

	val, err := s.Get(ctx, "TOKEN")
	if err != nil || string(val) != "abc" {
		t.Fatalf("Get = (%q, %v), want (abc, nil)", val, err)
	}

	if setErr := s.Set(ctx, "k", []byte("v")); !errors.Is(setErr, secrets.ErrNotSupported) {
		t.Fatalf("Set = %v, want ErrNotSupported", setErr)
	}

	if delErr := s.Delete(ctx, "k"); !errors.Is(delErr, secrets.ErrNotSupported) {
		t.Fatalf("Delete = %v, want ErrNotSupported", delErr)
	}

	keys, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	found := false

	for _, k := range keys {
		if k == "TOKEN" {
			found = true
		}
	}

	if !found {
		t.Fatalf("List() = %v, want TOKEN", keys)
	}

	if _, err := s.Get(ctx, "NOPE_DEF_MISSING_XYZ"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrNotFound", err)
	}
}

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
