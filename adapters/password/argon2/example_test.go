package argon2_test

import (
	"github.com/zenta-dev/zever/adapters/password/argon2"
	"github.com/zenta-dev/zever/core/password"
)

// Example opens the argon2id hasher with default options.
func Example() {
	hasher, err := argon2.New(password.Options{})
	if err != nil {
		return
	}
	_ = hasher
}
