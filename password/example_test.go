package password_test

import (
	"github.com/zenta-dev/zever/password"
	"github.com/zenta-dev/zever/password/argon2"
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
