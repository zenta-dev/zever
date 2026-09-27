package secrets_test

import (
	"context"

	secretsenv "github.com/zenta-dev/zever/adapters/secrets/env"
	"github.com/zenta-dev/zever/core/secrets"
)

// ExampleOpen opens the env secrets backend scoped to one prefix.
func ExampleOpen() {
	_ = secrets.Register(secrets.Env, func(o secrets.Options) (secrets.Secrets, error) {
		return secretsenv.New(secretsenv.Options{Prefix: o.Prefix})
	})

	s, err := secrets.Open(secrets.Env, secrets.Options{Prefix: "EXAMPLE"})
	if err != nil {
		return
	}

	defer func() { _ = s.Close(context.Background()) }()
}
