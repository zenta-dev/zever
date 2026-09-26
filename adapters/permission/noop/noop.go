package noop

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/permission"
)

var _ permission.Checker = (*checker)(nil)

type checker struct{}

// New builds a permission.Checker that denies all access with implicit_deny.
func New(opts permission.Options) (permission.Checker, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("noop: invalid options: %w", err)
	}
	return &checker{}, nil
}

func (c *checker) Can(_ context.Context, _ permission.Subject, _ string, _ permission.Resource) (permission.Decision, error) {
	return permission.Decision{Allowed: false, Reason: "implicit_deny"}, nil
}
