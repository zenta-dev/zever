package secrets_test

import (
	"context"

	secretsenv "github.com/zenta-dev/zever/adapters/secrets/env"
)

// Example opens the env secrets backend scoped to one prefix.
func Example() {
	s, err := secretsenv.New(secretsenv.Options{Prefix: "EXAMPLE"})
	if err != nil {
		return
	}

	defer func() { _ = s.Close(context.Background()) }()
}
