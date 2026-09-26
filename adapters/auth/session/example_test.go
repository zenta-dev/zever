package session_test

import (
	"context"
	"fmt"
	"time"

	authsession "github.com/zenta-dev/zever/adapters/auth/session"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	"github.com/zenta-dev/zever/core/auth"
	coresession "github.com/zenta-dev/zever/core/session"
)

// ExampleOpen opens the session backend, issues a token and verifies it.
func ExampleOpen() {
	const exampleAdapter auth.Adapter = "example-test"

	store, err := sessionmemory.New(coresession.Options{})
	if err != nil {
		fmt.Println("store error")
		return
	}

	_ = auth.Register(exampleAdapter, authsession.New)

	backend, err := auth.Open(exampleAdapter, auth.Options{Session: auth.SessionOptions{Store: store}})
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
