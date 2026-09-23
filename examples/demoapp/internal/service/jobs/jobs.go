// Package jobs implements the background jobs declared in
// schema/app.zen: SendWelcomeEmail, ProcessOrder, GenerateDailyReport and
// ReindexSearch.
package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/db"
	gen "github.com/zenta-dev/zever/examples/demoapp/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/mailer"
	"github.com/zenta-dev/zever/notification"
	"github.com/zenta-dev/zever/orm"
)

// Deps carries the resolved services job handlers need. It keeps the global
// job registry free of hidden state: the worker builds one Deps and closes
// over it when registering handlers.
type Deps struct {
	DB       db.DB
	Mailer   mailer.Mailer
	Notifier notification.Notifier
	Logger   log.Logger
}

// SendWelcomeEmailArgs is the payload of the SendWelcomeEmail job.
type SendWelcomeEmailArgs struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// ProcessOrderArgs is the payload of the ProcessOrder job declared in the
// schema.
type ProcessOrderArgs struct {
	OrderID string `json:"order_id"`
}

// RunWelcomeEmail sends the welcome mail and pushes a welcome notification.
func RunWelcomeEmail(ctx context.Context, deps Deps, email, name string) error {
	if email == "" {
		return errors.New("[jobs] welcome: email is empty")
	}
	if name == "" {
		name = email
	}

	mail := mailer.NewMail(
		mailer.Address{Address: "welcome@example.com", Name: "Demoapp"},
		[]mailer.Address{{Address: email, Name: name}},
		"Welcome to Demoapp",
		"Hi "+name+", welcome to the Zever demo!",
	)
	if err := deps.Mailer.Send(ctx, &mail); err != nil {
		return fmt.Errorf("[jobs] welcome: send mail: %w", err)
	}

	note := notification.NewNotification(email, notification.ChannelPush, "Welcome to Demoapp")
	note.Title = "Welcome"
	if err := deps.Notifier.Notify(ctx, &note); err != nil {
		return fmt.Errorf("[jobs] welcome: notify: %w", err)
	}

	deps.Logger.Info().Str("email", email).Msg("welcome email sent")
	return nil
}

// HandleSendWelcomeEmail returns the job.Register-compatible handler for
// SendWelcomeEmail.
func HandleSendWelcomeEmail(deps Deps) func(ctx context.Context, args SendWelcomeEmailArgs) error {
	return func(ctx context.Context, args SendWelcomeEmailArgs) error {
		return RunWelcomeEmail(ctx, deps, args.Email, args.Name)
	}
}

// RunProcessOrder marks a pending order paid. It stands in for the real
// payment capture the payment battery would perform.
func RunProcessOrder(ctx context.Context, deps Deps, orderID string) error {
	if orderID == "" {
		return errors.New("[jobs] process: order id is empty")
	}

	n, err := orm.UpdateTable(gen.Orders).Where(gen.OrderCols.ID.Eq(orderID)).Set(
		orm.Set(gen.OrderCols.Status, "paid"),
	).Exec(ctx, deps.DB)
	if err != nil {
		return fmt.Errorf("[jobs] process: mark paid: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("[jobs] process: order %q not found", orderID)
	}

	deps.Logger.Info().Str("order_id", orderID).Msg("order marked paid")
	return nil
}

// HandleProcessOrder returns the job.Register-compatible handler for
// ProcessOrder.
func HandleProcessOrder(deps Deps) func(ctx context.Context, args ProcessOrderArgs) error {
	return func(ctx context.Context, args ProcessOrderArgs) error {
		return RunProcessOrder(ctx, deps, args.OrderID)
	}
}

// RunReindex counts products as a stand-in for search-index maintenance.
func RunReindex(ctx context.Context, deps Deps) error {
	items, err := orm.From(gen.Products).All(ctx, deps.DB)
	if err != nil {
		return fmt.Errorf("[jobs] reindex: list products: %w", err)
	}
	deps.Logger.Info().Int("products", len(items)).Msg("search index refreshed")
	return nil
}

// HandleReindexSearch returns the job.Register-compatible handler for
// ReindexSearch.
func HandleReindexSearch(deps Deps) func(ctx context.Context, args struct{}) error {
	return func(ctx context.Context, _ struct{}) error {
		return RunReindex(ctx, deps)
	}
}

// RunDailyReport counts pending orders as a stand-in for the daily report.
func RunDailyReport(ctx context.Context, deps Deps) error {
	orders, err := orm.From(gen.Orders).Where(gen.OrderCols.Status.Eq("pending")).All(ctx, deps.DB)
	if err != nil {
		return fmt.Errorf("[jobs] report: list orders: %w", err)
	}
	deps.Logger.Info().Int("pending_orders", len(orders)).Msg("daily report generated")
	return nil
}

// HandleGenerateDailyReport returns the job.Register-compatible handler for
// GenerateDailyReport.
func HandleGenerateDailyReport(deps Deps) func(ctx context.Context, args struct{}) error {
	return func(ctx context.Context, _ struct{}) error {
		return RunDailyReport(ctx, deps)
	}
}
