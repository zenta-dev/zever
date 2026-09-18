package authz_test

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/authz"
	"github.com/zenta-dev/zever/permission"
)

type openAuth struct{}

func (openAuth) Issue(_ context.Context, _ string, _ map[string]any, _ time.Duration) (auth.Token, error) {
	return auth.Token{}, nil
}

func (openAuth) Verify(_ context.Context, token string) (auth.Claims, error) {
	if token != "good" {
		return auth.Claims{}, auth.ErrInvalidToken
	}
	return auth.Claims{Subject: "user-1"}, nil
}

func (openAuth) Revoke(_ context.Context, _ string) error { return nil }
func (openAuth) Close() error                             { return nil }

type openChecker struct{}

func (openChecker) Can(_ context.Context, _ permission.Subject, _ string, _ permission.Resource) (permission.Decision, error) {
	return permission.Decision{Allowed: true}, nil
}

// ExampleOpen authorizes a request with stub backends and prints the subject.
func ExampleOpen() {
	claims, err := authz.Authorize(
		context.Background(),
		openAuth{},
		openChecker{},
		authz.Policy{AuthRequired: true, PermissionCheck: "read", ResourceType: "doc"},
		"good",
		"doc-1",
	)
	if err != nil {
		fmt.Println("deny")
		return
	}

	fmt.Println(claims.Subject)
	// Output: user-1
}
