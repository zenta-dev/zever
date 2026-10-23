package billing

import (
	"context"
	"sync/atomic"
)

var testSeq atomic.Int64

type stubBilling struct {
	customer     Customer
	subscription Subscription
	invoice      Invoice
}

func (s *stubBilling) CreateCustomer(_ context.Context, name, email, _ string) (Customer, error) {
	c := s.customer
	c.Name = name
	c.Email = email

	return c, nil
}

func (s *stubBilling) CreateSubscription(_ context.Context, customerID, planID, _ string) (Subscription, error) {
	sub := s.subscription
	sub.CustomerID = customerID
	sub.PlanID = planID

	return sub, nil
}

func (s *stubBilling) CancelSubscription(_ context.Context, _ string) error {
	return nil
}

func (s *stubBilling) GetInvoice(_ context.Context, _ string) (Invoice, error) {
	return s.invoice, nil
}

func (s *stubBilling) Close() error {
	return nil
}
