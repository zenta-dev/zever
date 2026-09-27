package crypto_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/adapters/crypto/local"
	"github.com/zenta-dev/zever/core/crypto"
)

// ExampleOpen opens the local adapter and round-trips a secret.
func ExampleOpen() {
	if err := crypto.Register(crypto.AdapterLocal, local.New); err != nil {
		return
	}

	enc, err := crypto.Open(crypto.AdapterLocal, crypto.Options{
		Key: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=",
	})
	if err != nil {
		return
	}

	ctx := context.Background()

	sealed, err := enc.Encrypt(ctx, []byte("hello"))
	if err != nil {
		return
	}

	plain, err := enc.Decrypt(ctx, sealed)
	if err != nil {
		return
	}

	fmt.Println(string(plain))
	// Output: hello
}
