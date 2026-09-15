package stub

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/zenta-dev/zever/billing"
)

var _ billing.Billing = (*driver)(nil)

// New creates an in-memory stub billing backend.
func New() billing.Billing {
	return &driver{}
}

// Open validates options and returns a stub billing backend.
func Open(o billing.Options) (billing.Billing, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("stub: %w", err)
	}

	return New(), nil
}

type driver struct {
	mu            sync.Mutex
	customers     map[string]billing.Customer
	subscriptions map[string]billing.Subscription
	invoices      map[string][]billing.Invoice
}

func (d *driver) CreateCustomer(_ context.Context, name string, email string) (billing.Customer, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.customers == nil {
		d.customers = make(map[string]billing.Customer)
	}

	id := "cus_" + newULID()
	c := billing.Customer{ID: id, Name: name, Email: email}
	d.customers[id] = c

	return c, nil
}

func (d *driver) CreateSubscription(_ context.Context, customerID string, planID string) (billing.Subscription, error) {
	if customerID == "" {
		return billing.Subscription{}, fmt.Errorf("stub: create subscription: %w", billing.ErrMissingCustomerID)
	}

	if planID == "" {
		return billing.Subscription{}, fmt.Errorf("stub: create subscription: %w", billing.ErrMissingPlanID)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.customers[customerID]; !ok {
		notFound := error(&billing.NotFoundError{Resource: "customer", ID: customerID})
		return billing.Subscription{}, fmt.Errorf("stub: create subscription: %w", notFound)
	}

	if d.subscriptions == nil {
		d.subscriptions = make(map[string]billing.Subscription)
	}

	id := "sub_" + newULID()
	s := billing.Subscription{ID: id, CustomerID: customerID, PlanID: planID, Status: billing.SubscriptionActive}
	d.subscriptions[id] = s

	if d.invoices == nil {
		d.invoices = make(map[string][]billing.Invoice)
	}

	inv := billing.Invoice{ID: "inv_" + newULID(), AmountDue: 0, Currency: "usd", Status: billing.InvoicePaid}
	d.invoices[customerID] = append(d.invoices[customerID], inv)

	return s, nil
}

func (d *driver) CancelSubscription(_ context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("stub: cancel subscription: %w", billing.ErrMissingSubscriptionID)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	s, ok := d.subscriptions[id]
	if !ok {
		notFound := error(&billing.NotFoundError{Resource: "subscription", ID: id})
		return fmt.Errorf("stub: cancel subscription: %w", notFound)
	}

	s.Status = billing.SubscriptionCanceled
	d.subscriptions[id] = s

	return nil
}

func (d *driver) GetInvoice(_ context.Context, customerID string) (billing.Invoice, error) {
	if customerID == "" {
		return billing.Invoice{}, fmt.Errorf("stub: get invoice: %w", billing.ErrMissingCustomerID)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	invs, ok := d.invoices[customerID]
	if !ok || len(invs) == 0 {
		notFound := error(&billing.NotFoundError{Resource: "invoice", ID: customerID})
		return billing.Invoice{}, fmt.Errorf("stub: get invoice: %w", notFound)
	}

	return invs[len(invs)-1], nil
}

// Close releases stub resources.
func (d *driver) Close() error {
	return nil
}

func newULID() string {
	return newULIDFromReader(cryptorand.Reader)
}

func newULIDFromReader(r io.Reader) string {
	id, err := ulid.New(ulid.Timestamp(time.Now()), r)
	if err != nil {
		return ulid.Make().String()
	}

	return id.String()
}
