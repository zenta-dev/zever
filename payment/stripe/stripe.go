package stripe

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v82"

	"github.com/zenta-dev/zever/payment"
)

// driver is the Stripe payment backend.
type driver struct {
	// client talks to the Stripe API.
	client *stripe.Client
	// httpClient backs the Stripe backend transport.
	httpClient *http.Client
	// webhookSecret verifies webhook signatures.
	webhookSecret string
	// maxWebhookBytes bounds webhook payloads.
	maxWebhookBytes int
}

// New validates o then returns a Stripe backend matching Factory.
func New(o payment.Options) (payment.Payment, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("stripe: %w", err)
	}

	if o.SecretKey == "" {
		return nil, ErrMissingSecretKey
	}

	if o.WebhookSecret == "" {
		return nil, ErrMissingWebhookSecret
	}

	httpClient := &http.Client{Timeout: time.Duration(payment.DefaultHTTPTimeout) * time.Second}

	cfg := &stripe.BackendConfig{HTTPClient: httpClient}
	if o.Endpoint != "" {
		cfg.URL = stripe.String(o.Endpoint)
	}

	client := stripe.NewClient(o.SecretKey, stripe.WithBackends(stripe.NewBackendsWithConfig(cfg)))

	maxBytes := o.MaxWebhookBytes
	if maxBytes <= 0 {
		maxBytes = payment.DefaultMaxWebhookBytes
	}

	return &driver{client: client, httpClient: httpClient, webhookSecret: o.WebhookSecret, maxWebhookBytes: maxBytes}, nil
}

// CreatePayment creates a Stripe PaymentIntent from req.
func (d *driver) CreatePayment(ctx context.Context, req payment.Request) (payment.Result, error) {
	if req.Amount <= 0 {
		return payment.Result{}, fmt.Errorf("stripe: create: %w", payment.ErrInvalidAmount)
	}

	currency := strings.ToLower(req.Currency)
	if currency == "" {
		return payment.Result{}, fmt.Errorf("stripe: create: %w", payment.ErrMissingCurrency)
	}

	params := &stripe.PaymentIntentCreateParams{
		Amount:   stripe.Int64(req.Amount),
		Currency: stripe.String(currency),
	}
	if len(req.Meta) > 0 {
		params.Metadata = make(map[string]string, len(req.Meta))
		for k, v := range req.Meta {
			params.Metadata[k] = v
		}
	}

	if k := req.Meta["idempotency_key"]; k != "" {
		params.SetIdempotencyKey(k)
	}

	pi, err := d.client.V1PaymentIntents.Create(ctx, params)
	if err != nil {
		return payment.Result{}, fmt.Errorf("stripe: create: %w", err)
	}

	return payment.Result{
		ID:       pi.ID,
		Status:   payment.PaymentStatus(string(pi.Status)),
		Amount:   pi.Amount,
		Currency: string(pi.Currency),
	}, nil
}

// Refund creates a Stripe refund of amount against payment id.
func (d *driver) Refund(ctx context.Context, id string, amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("stripe: refund: %w", payment.ErrInvalidAmount)
	}

	if id == "" {
		return fmt.Errorf("stripe: refund: %w", payment.ErrMissingPaymentID)
	}

	params := &stripe.RefundCreateParams{
		PaymentIntent: stripe.String(id),
		Amount:        stripe.Int64(amount),
	}

	if _, err := d.client.V1Refunds.Create(ctx, params); err != nil {
		return fmt.Errorf("stripe: refund: %w", err)
	}

	return nil
}

// GetPayment fetches a Stripe PaymentIntent by id.
func (d *driver) GetPayment(ctx context.Context, id string) (payment.Result, error) {
	if id == "" {
		return payment.Result{}, fmt.Errorf("stripe: get: %w", payment.ErrMissingPaymentID)
	}

	pi, err := d.client.V1PaymentIntents.Retrieve(ctx, id, &stripe.PaymentIntentRetrieveParams{})
	if err != nil {
		return payment.Result{}, fmt.Errorf("stripe: get: %w", err)
	}

	return payment.Result{
		ID:       pi.ID,
		Status:   payment.PaymentStatus(string(pi.Status)),
		Amount:   pi.Amount,
		Currency: string(pi.Currency),
	}, nil
}

// WebhookEvent verifies signature then decodes a Stripe webhook payload.
func (d *driver) WebhookEvent(_ context.Context, raw []byte, signature string) (payment.Event, error) {
	if d.maxWebhookBytes <= 0 {
		return payment.Event{}, fmt.Errorf("stripe: webhook: invalid max size %d", d.maxWebhookBytes)
	}

	if len(raw) > d.maxWebhookBytes {
		sizeErr := error(&payment.SizeLimitError{Size: len(raw), Limit: d.maxWebhookBytes})
		return payment.Event{}, fmt.Errorf("stripe: webhook: %w", sizeErr)
	}

	if d.webhookSecret == "" {
		return payment.Event{}, ErrMissingWebhookSecret
	}

	event, err := d.client.ConstructEvent(raw, signature, d.webhookSecret)
	if err != nil {
		return payment.Event{}, fmt.Errorf("stripe: webhook: %w", err)
	}

	ev := payment.Event{Type: payment.EventType(string(event.Type))}

	switch {
	case strings.HasPrefix(string(event.Type), "payment_intent."):
		var pi stripe.PaymentIntent
		if err := payment.LimitDecode(event.Data.Raw, &pi, d.maxWebhookBytes); err != nil {
			return payment.Event{}, fmt.Errorf("stripe: webhook: unmarshal: %w", err)
		}

		ev.Object = payment.Result{
			ID:       pi.ID,
			Status:   payment.PaymentStatus(string(pi.Status)),
			Amount:   pi.Amount,
			Currency: string(pi.Currency),
		}
	case event.Type == stripe.EventTypeChargeRefunded:
		var ch stripe.Charge
		if err := payment.LimitDecode(event.Data.Raw, &ch, d.maxWebhookBytes); err != nil {
			return payment.Event{}, fmt.Errorf("stripe: webhook: unmarshal: %w", err)
		}

		objectID := ch.ID
		if ch.PaymentIntent != nil && ch.PaymentIntent.ID != "" {
			objectID = ch.PaymentIntent.ID
		}

		refunded := ch.AmountRefunded

		if ch.Refunds != nil && len(ch.Refunds.Data) > 0 {
			if tail := ch.Refunds.Data[len(ch.Refunds.Data)-1]; tail != nil {
				refunded = tail.Amount
			}
		}

		ev.Object = payment.Result{
			ID:       objectID,
			Status:   payment.PaymentStatus(string(ch.Status)),
			Amount:   refunded,
			Currency: string(ch.Currency),
		}
	}

	return ev, nil
}

// Close releases backend resources.
func (d *driver) Close() error {
	return nil
}
