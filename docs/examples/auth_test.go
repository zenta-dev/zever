package examples_test

import (
	"context"
	"fmt"
	"time"

	authsession "github.com/zenta-dev/zever/adapters/auth/session"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	"github.com/zenta-dev/zever/core/auth"
	coresession "github.com/zenta-dev/zever/core/session"
)

// ExampleAuth_issueVerify mirrors the session-auth doc sample: build an
// in-memory session store, open the session-backed auth adapter on it,
// issue a token, and verify it back to its claims.
func Example_authIssueVerify() {
	store, err := sessionmemory.New(coresession.Options{})
	if err != nil {
		fmt.Println("store error")
		return
	}
	defer func() { _ = store.Close() }()

	backend, err := authsession.New(auth.Options{Session: auth.SessionOptions{Store: store}})
	if err != nil {
		fmt.Println("open error")
		return
	}
	defer func() { _ = backend.Close() }()

	ctx := context.Background()
	token, err := backend.Issue(ctx, "user-1", map[string]any{"role": "member"}, time.Hour)
	if err != nil {
		fmt.Println("issue error")
		return
	}
	claims, err := backend.Verify(ctx, token.Value)
	if err != nil {
		fmt.Println("verify error")
		return
	}
	fmt.Println(claims.Subject, claims.Custom["role"])
	// Output: user-1 member
}
