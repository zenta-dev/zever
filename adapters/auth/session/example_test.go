package auth_test

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/auth"
	authsession "github.com/zenta-dev/zever/auth/session"
)

// ExampleOpen opens the session backend, issues a token and verifies it.
func ExampleOpen() {
	const exampleAdapter auth.Adapter = 32002

	_ = auth.Register(exampleAdapter, authsession.New)

	backend, err := auth.Open(exampleAdapter, auth.Options{})
	if err != nil {
		fmt.Println("open error")
		return
	}
	defer backend.Close()

	ctx := context.Background()
	token, err := backend.Issue(ctx, "user-1", nil, time.Hour)
	if err != nil {
		fmt.Println("issue error")
		return
	}

	claims, err := backend.Verify(ctx, token.Value)
	if err != nil {
		fmt.Println("verify error")
		return
	}

	fmt.Println(claims.Subject)
	// Output: user-1
}
