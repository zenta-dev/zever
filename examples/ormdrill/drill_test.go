package ormdrill

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/db"
)

// runDemo opens a fresh shop database, executes one demo, and prints any
// error. It replaces the per-Example open/run/close boilerplate.
func runDemo(demo func(ctx context.Context, conn db.DB) error) {
	ctx := context.Background()

	conn, openErr := OpenShop(ctx)
	if openErr != nil {
		fmt.Println("error:", openErr)

		return
	}

	defer func() { _ = conn.Close(ctx) }()

	if err := demo(ctx, conn); err != nil {
		fmt.Println("error:", err)
	}
}
