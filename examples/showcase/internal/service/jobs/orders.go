package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/notification"
	gen "github.com/zenta-dev/zever/examples/showcase/generated/zenorm/orm/gen/shop"
	"github.com/zenta-dev/zever/orm"
)

// SendConfirmationArgs is the payload of the SendConfirmation job declared
// in the schema.
type SendConfirmationArgs struct {
	OrderID string `json:"order_id"`
}

// ProcessOrderArgs is the payload of the ProcessOrder job declared in the
// schema.
type ProcessOrderArgs struct {
	OrderID string `json:"order_id"`
}

// RunConfirmation loads the order, sends a mail receipt to the buyer and
// pushes a buyer notification.
func RunConfirmation(ctx context.Context, deps Deps, orderID string) error {
	if orderID == "" {
		return errors.New("[jobs] confirmation: order id is empty")
	}

	order, ok, err := orm.From(gen.Orders).Where(gen.OrderCols.ID.Eq(orderID)).First(ctx, deps.DB)
	if err != nil {
		return fmt.Errorf("[jobs] confirmation: lookup order: %w", err)
	}
	if !ok {
		return fmt.Errorf("[jobs] confirmation: order %q not found", orderID)
	}

	buyer, ok, err := orm.From(gen.Users).Where(gen.UserCols.ID.Eq(order.UserID)).First(ctx, deps.DB)
	if err != nil {
		return fmt.Errorf("[jobs] confirmation: lookup buyer: %w", err)
	}
	if !ok {
		return fmt.Errorf("[jobs] confirmation: buyer %q not found", order.UserID)
	}

	body := fmt.Sprintf("Order %s for %d cents is %s.", order.ID, order.TotalCents, order.Status)

	mail := mailer.NewMail(
		mailer.Address{Address: "receipts@example.com", Name: "Showcase"},
		[]mailer.Address{{Address: buyer.Email}},
		fmt.Sprintf("Order %s confirmed", order.ID),
		body,
	)
	if merr := deps.Mailer.Send(ctx, &mail); merr != nil {
		return fmt.Errorf("[jobs] confirmation: send mail: %w", merr)
	}

	note := notification.NewNotification(buyer.Email, notification.ChannelPush, body)
	note.Title = "Order confirmed"
	note.Data = map[string]string{"order_id": order.ID}
	if nerr := deps.Notifier.Notify(ctx, &note); nerr != nil {
		return fmt.Errorf("[jobs] confirmation: notify buyer: %w", nerr)
	}

	deps.Logger.Info().Str("order_id", order.ID).Msg("confirmation sent")
	return nil
}

// HandleSendConfirmation returns the job.Register-compatible handler for
// SendConfirmation.
func HandleSendConfirmation(deps Deps) func(ctx context.Context, args SendConfirmationArgs) error {
	return func(ctx context.Context, args SendConfirmationArgs) error {
		return RunConfirmation(ctx, deps, args.OrderID)
	}
}

// RunProcessOrder marks a pending order paid. It stands in for the real
// payment capture the payment battery would perform.
func RunProcessOrder(ctx context.Context, deps Deps, orderID string) error {
	if orderID == "" {
		return errors.New("[jobs] process: order id is empty")
	}

	n, err := orm.UpdateTable(gen.Orders).Where(gen.OrderCols.ID.Eq(orderID)).Set(
		orm.Set(gen.OrderCols.Status, gen.OrderStatusPaid),
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
	deps.Logger.Info().Int("products", len(items)).Msg("search reindexed")
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
	orders, err := orm.From(gen.Orders).Where(gen.OrderCols.Status.Eq(gen.OrderStatusPending)).All(ctx, deps.DB)
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
