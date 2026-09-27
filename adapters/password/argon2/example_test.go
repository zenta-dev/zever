package argon2_test

import (
	"github.com/zenta-dev/zever/adapters/password/argon2"
	"github.com/zenta-dev/zever/core/password"
)

// ExampleOpen opens the argon2id hasher with default options.
func ExampleOpen() {
	_ = password.Register(password.AdapterArgon2ID, argon2.New)

	hasher, err := password.Open(password.AdapterArgon2ID, password.Options{})
	if err != nil {
		return
	}
	_ = hasher
}
