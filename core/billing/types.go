package billing

// Customer represents a billing customer.
type Customer struct {
	// ID is the customer identifier.
	ID string
	// Name is the customer display name.
	Name string
	// Email is the customer email address.
	Email string
}

// Subscription represents a billing subscription.
type Subscription struct {
	// ID is the subscription identifier.
	ID string
	// CustomerID is the owning customer identifier.
	CustomerID string
	// PlanID is the subscribed plan identifier.
	PlanID string
	// Status is the subscription lifecycle status.
	Status SubscriptionStatus
}

// Invoice represents a billing invoice.
type Invoice struct {
	// ID is the invoice identifier.
	ID string
	// AmountDue is the amount due in minor units.
	AmountDue int64
	// Currency is the invoice currency code.
	Currency string
	// Status is the invoice lifecycle status.
	Status InvoiceStatus
}

// SubscriptionStatus represents the status of a subscription.
type SubscriptionStatus string

// Subscription lifecycle statuses.
const (
	// SubscriptionActive marks an active subscription.
	SubscriptionActive SubscriptionStatus = "active"
	// SubscriptionCanceled marks a canceled subscription.
	SubscriptionCanceled SubscriptionStatus = "canceled"
	// SubscriptionPastDue marks a past-due subscription.
	SubscriptionPastDue SubscriptionStatus = "past_due"
	// SubscriptionTrial marks a trial subscription.
	SubscriptionTrial SubscriptionStatus = "trial"
)

// InvoiceStatus represents the status of an invoice.
type InvoiceStatus string

// Invoice lifecycle statuses.
const (
	// InvoicePaid marks a paid invoice.
	InvoicePaid InvoiceStatus = "paid"
	// InvoiceDraft marks a draft invoice.
	InvoiceDraft InvoiceStatus = "draft"
	// InvoiceFinalized marks a finalized invoice.
	InvoiceFinalized InvoiceStatus = "finalized"
	// InvoiceOpen marks an open invoice.
	InvoiceOpen InvoiceStatus = "open"
)
