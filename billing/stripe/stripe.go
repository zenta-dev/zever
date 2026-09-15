package stripe

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stripe/stripe-go/v82"

	"github.com/zenta-dev/zever/billing"
)

var _ billing.Billing = (*driver)(nil)

type driver struct {
	client *stripe.Client
}

// Open creates a Stripe billing adapter from the given options.
func Open(o billing.Options) (billing.Billing, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("stripe: %w", err)
	}

	if o.SecretKey == "" {
		return nil, ErrMissingSecretKey
	}

	cfg := &stripe.BackendConfig{HTTPClient: &http.Client{Timeout: billing.DefaultHTTPTimeout}}
	if o.Endpoint != "" {
		cfg.URL = stripe.String(o.Endpoint)
	}

	client := stripe.NewClient(o.SecretKey, stripe.WithBackends(stripe.NewBackendsWithConfig(cfg)))

	return &driver{client: client}, nil
}

func (d *driver) CreateCustomer(ctx context.Context, name string, email string) (billing.Customer, error) {
	params := &stripe.CustomerCreateParams{Name: stripe.String(name), Email: stripe.String(email)}

	cus, err := d.client.V1Customers.Create(ctx, params)
	if err != nil {
		return billing.Customer{}, fmt.Errorf("stripe: create customer: %w", err)
	}

	return billing.Customer{ID: cus.ID, Name: cus.Name, Email: cus.Email}, nil
}

func (d *driver) CreateSubscription(ctx context.Context, customerID string, planID string) (billing.Subscription, error) {
	if customerID == "" {
		return billing.Subscription{}, fmt.Errorf("stripe: create subscription: %w", billing.ErrMissingCustomerID)
	}

	if planID == "" {
		return billing.Subscription{}, fmt.Errorf("stripe: create subscription: %w", billing.ErrMissingPlanID)
	}

	params := &stripe.SubscriptionCreateParams{
		Customer: stripe.String(customerID),
		Items: []*stripe.SubscriptionCreateItemParams{
			{Price: stripe.String(planID)},
		},
	}

	sub, err := d.client.V1Subscriptions.Create(ctx, params)
	if err != nil {
		return billing.Subscription{}, fmt.Errorf("stripe: create subscription: %w", err)
	}

	return billing.Subscription{
		ID:         sub.ID,
		CustomerID: customerID,
		PlanID:     planID,
		Status:     billing.SubscriptionStatus(string(sub.Status)),
	}, nil
}

func (d *driver) CancelSubscription(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("stripe: cancel subscription: %w", billing.ErrMissingSubscriptionID)
	}

	_, err := d.client.V1Subscriptions.Cancel(ctx, id, nil)
	if err != nil {
		return fmt.Errorf("stripe: cancel subscription: %w", err)
	}

	return nil
}

func (d *driver) GetInvoice(ctx context.Context, customerID string) (billing.Invoice, error) {
	if customerID == "" {
		return billing.Invoice{}, fmt.Errorf("stripe: list invoices: %w", billing.ErrMissingCustomerID)
	}

	params := &stripe.InvoiceListParams{Customer: stripe.String(customerID), Limit: stripe.Int64(100)}
	for inv, err := range d.client.V1Invoices.List(ctx, params) {
		if err != nil {
			return billing.Invoice{}, fmt.Errorf("stripe: list invoices: %w", err)
		}

		if inv.Status == stripe.InvoiceStatusDraft || inv.Status == stripe.InvoiceStatusVoid {
			continue
		}

		return billing.Invoice{
			ID:        inv.ID,
			AmountDue: inv.AmountDue,
			Currency:  string(inv.Currency),
			Status:    billing.InvoiceStatus(string(inv.Status)),
		}, nil
	}

	// Wrap via error interface to satisfy go vet printf check
	// (direct %w of *NotFoundError triggers false-positive defeats-errors.Is).
	nf := error(&billing.NotFoundError{Resource: "invoice", ID: customerID})
	return billing.Invoice{}, fmt.Errorf("stripe: list invoices: %w", nf)
}

// Close releases Stripe resources.
func (d *driver) Close() error {
	return nil
}
