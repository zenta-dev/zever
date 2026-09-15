package payment

// PaymentStatus represents the status of a payment.
//
//nolint:revive // exported name kept stable for API compatibility
type PaymentStatus string

// Terminal and in-flight states a payment can be in.
const (
	// PaymentSucceeded marks a captured, completed payment.
	PaymentSucceeded PaymentStatus = "succeeded"
	// PaymentPending marks a payment awaiting completion.
	PaymentPending PaymentStatus = "pending"
	// PaymentFailed marks a failed payment.
	PaymentFailed PaymentStatus = "failed"
	// PaymentPartiallyRefunded marks a payment with a partial refund applied.
	PaymentPartiallyRefunded PaymentStatus = "partially_refunded"
	// PaymentRefunded marks a fully refunded payment.
	PaymentRefunded PaymentStatus = "refunded"
)

// PaymentMethod represents a payment method type.
//
//nolint:revive // exported name kept stable for API compatibility
type PaymentMethod string

// Supported payment method types.
const (
	// MethodCard is card payment.
	MethodCard PaymentMethod = "card"
	// MethodBankTransfer is bank transfer payment.
	MethodBankTransfer PaymentMethod = "bank_transfer"
)

// EventType represents the type of a payment webhook event.
// Values are provider-defined (e.g. "payment_intent.succeeded",
// "transaction.completed"); adapters pass them through verbatim.
type EventType string

// Request contains the details for creating a payment.
// Amount is in minor units (cents). Meta carries provider-specific
// fields: "price_id" and "customer_id" (paddle), "idempotency_key"
// (stripe), "transit_id" (paddle request linking).
type Request struct {
	Amount   int64
	Currency string
	Method   PaymentMethod
	Meta     map[string]string
}

// Result represents the result of a payment operation.
type Result struct {
	ID       string
	Status   PaymentStatus
	Amount   int64
	Currency string
}

// Event represents an incoming webhook event.
type Event struct {
	Type   EventType
	Object Result
}
