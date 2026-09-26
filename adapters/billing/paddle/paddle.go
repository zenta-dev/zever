package paddle

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/PaddleHQ/paddle-go-sdk/v5"

	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/shared/httpclient"
	"github.com/zenta-dev/zever/shared/providersopt"
)

// newSDK creates the underlying Paddle SDK client. It is a seam for tests.
var newSDK = paddle.New

type driver struct {
	client *paddle.SDK
}

// New creates a Paddle billing adapter from the given options.
func New(o billing.Options) (billing.Billing, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("paddle: %w", err)
	}

	if o.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	endpoint := providersopt.PaddleEndpoint(o.Endpoint, o.Sandbox)

	client, err := newSDK(o.APIKey, paddle.WithBaseURL(endpoint), paddle.WithClient(httpclient.NewClient(billing.DefaultHTTPTimeout)))
	if err != nil {
		return nil, fmt.Errorf("paddle: %w", err)
	}

	return &driver{client: client}, nil
}

// CreateCustomer creates a Paddle customer. idempotencyKey is accepted for
// billing.Billing interface compatibility but ignored: paddle-go-sdk/v5
// exposes no request-level idempotency mechanism.
func (d *driver) CreateCustomer(ctx context.Context, name, email, _ string) (billing.Customer, error) {
	req := &paddle.CreateCustomerRequest{Email: email}
	if name != "" {
		req.Name = paddle.PtrTo(name)
	}

	cus, err := d.client.CreateCustomer(ctx, req)
	if err != nil {
		return billing.Customer{}, fmt.Errorf("paddle: create customer: %w", err)
	}

	return billing.Customer{ID: cus.ID, Name: derefStr(cus.Name), Email: cus.Email}, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// CreateSubscription creates a transaction in automatic collection mode for the
// given price. Paddle has no server-side subscription creation API — the
// transaction acts as the subscription record and does not auto-renew for
// future billing periods. idempotencyKey is accepted for billing.Billing
// interface compatibility but ignored: paddle-go-sdk/v5 exposes no
// request-level idempotency mechanism.
func (d *driver) CreateSubscription(ctx context.Context, customerID, planID, _ string) (billing.Subscription, error) {
	if customerID == "" {
		return billing.Subscription{}, billing.ErrMissingCustomerID
	}

	if planID == "" {
		return billing.Subscription{}, billing.ErrMissingPlanID
	}

	txn, err := d.client.CreateTransaction(ctx, &paddle.CreateTransactionRequest{
		Items: []paddle.CreateTransactionItems{
			*paddle.NewCreateTransactionItemsTransactionItemFromCatalog(&paddle.TransactionItemFromCatalog{
				PriceID:  planID,
				Quantity: 1,
			}),
		},
		CustomerID:     paddle.PtrTo(customerID),
		CollectionMode: paddle.PtrTo(paddle.CollectionModeAutomatic),
		Status:         paddle.PtrTo(paddle.TransactionStatusReady),
	})
	if err != nil {
		return billing.Subscription{}, fmt.Errorf("paddle: create subscription: %w", err)
	}

	return billing.Subscription{ID: txn.ID, CustomerID: customerID, PlanID: planID, Status: subscriptionStatus(txn.Status)}, nil
}

func subscriptionStatus(s paddle.TransactionStatus) billing.SubscriptionStatus {
	switch s { //nolint:exhaustive // default passes unmapped statuses (e.g. Draft) through as-is
	case paddle.TransactionStatusReady, paddle.TransactionStatusBilled,
		paddle.TransactionStatusPaid, paddle.TransactionStatusCompleted:
		return billing.SubscriptionActive
	case paddle.TransactionStatusPastDue:
		return billing.SubscriptionPastDue
	case paddle.TransactionStatusCanceled:
		return billing.SubscriptionCanceled
	default:
		return billing.SubscriptionStatus(string(s))
	}
}

func (d *driver) CancelSubscription(ctx context.Context, id string) error {
	if id == "" {
		return billing.ErrMissingSubscriptionID
	}

	_, err := d.client.UpdateTransaction(ctx, &paddle.UpdateTransactionRequest{
		TransactionID: id,
		Status:        paddle.NewPatchField(paddle.TransactionStatusCanceled),
	})
	if err != nil {
		return fmt.Errorf("paddle: cancel subscription: %w", err)
	}

	return nil
}

func (d *driver) GetInvoice(ctx context.Context, customerID string) (billing.Invoice, error) {
	if customerID == "" {
		return billing.Invoice{}, billing.ErrMissingCustomerID
	}

	col, err := d.client.ListTransactions(ctx, &paddle.ListTransactionsRequest{
		CustomerID: []string{customerID},
		Status: []string{
			string(paddle.TransactionStatusBilled),
			string(paddle.TransactionStatusPaid),
			string(paddle.TransactionStatusCompleted),
		},
		PerPage: paddle.PtrTo(1),
		OrderBy: paddle.PtrTo("billed_at[DESC]"),
	})
	if err != nil {
		return billing.Invoice{}, fmt.Errorf("paddle: list transactions: %w", err)
	}

	// Paddle's Collection.Iter handles pagination via nextURL/hasMore internally,
	// so iterating will fetch subsequent pages until HasMore==false if needed.
	var first *paddle.Transaction

	if iterErr := col.Iter(ctx, func(t *paddle.Transaction) (bool, error) {
		first = t
		return false, nil
	}); iterErr != nil {
		return billing.Invoice{}, fmt.Errorf("paddle: list transactions: %w", iterErr)
	}

	if first == nil {
		var notFound error = &billing.NotFoundError{Resource: "invoice", ID: customerID}
		return billing.Invoice{}, fmt.Errorf("paddle: list transactions: %w", notFound)
	}

	total, err := parsePaddleTotal(first.Details.Totals.Total, string(first.CurrencyCode))
	if err != nil {
		return billing.Invoice{}, fmt.Errorf("paddle: parse total: %w", err)
	}

	return billing.Invoice{
		ID:        first.ID,
		AmountDue: total,
		Currency:  string(first.CurrencyCode),
		Status:    billing.InvoiceStatus(string(first.Status)),
	}, nil
}

func parsePaddleTotal(s string, currency string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, billing.ErrEmptyAmount
	}

	// Paddle totals are decimal strings (e.g. "10.00" or "1000").
	// If a decimal point is present, parse via ParseMinorUnits to apply
	// currency exponent correctly; otherwise treat as minor units integer.
	if strings.Contains(s, ".") {
		return billing.ParseMinorUnits(s, currency)
	}

	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", billing.ErrMalformedAmount, s)
	}

	return v, nil
}

func (d *driver) Close() error {
	return nil
}
