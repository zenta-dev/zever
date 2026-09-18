package paddle

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PaddleHQ/paddle-go-sdk/v5"

	"github.com/zenta-dev/zever/internal/httpclient"
	"github.com/zenta-dev/zever/payment"
)

// newSDK constructs the Paddle SDK. It is a variable so tests can stub construction.
var newSDK = paddle.New

// driver is a payment.Payment backed by Paddle transactions and adjustments.
type driver struct {
	client          *paddle.SDK
	verifier        *paddle.WebhookVerifier
	webhookSecret   string
	maxWebhookBytes int
}

// Open creates a Payment backed by Paddle.
func Open(o payment.Options) (payment.Payment, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("paddle: %w", err)
	}

	if o.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	endpoint := o.Endpoint
	if endpoint == "" {
		if o.Sandbox {
			endpoint = paddle.SandboxBaseURL
		} else {
			endpoint = paddle.ProductionBaseURL
		}
	}

	client, err := newSDK(o.APIKey, paddle.WithBaseURL(endpoint), paddle.WithClient(httpclient.NewClient(payment.DefaultHTTPTimeout)))
	if err != nil {
		return nil, fmt.Errorf("paddle: %w", err)
	}

	maxWebhookBytes := o.MaxWebhookBytes
	if maxWebhookBytes <= 0 {
		maxWebhookBytes = payment.DefaultMaxWebhookBytes
	}

	return &driver{
		client: client,
		verifier: paddle.NewWebhookVerifier(
			o.WebhookSecret, paddle.VerifierWithTimestampTolerance(5*time.Minute),
		),
		webhookSecret:   o.WebhookSecret,
		maxWebhookBytes: maxWebhookBytes,
	}, nil
}

// CreatePayment creates a Paddle transaction for req.
func (d *driver) CreatePayment(ctx context.Context, req payment.Request) (payment.Result, error) {
	if req.Amount <= 0 {
		return payment.Result{}, fmt.Errorf("paddle: create: %w", payment.ErrInvalidAmount)
	}

	if req.Currency == "" {
		return payment.Result{}, fmt.Errorf("paddle: create: %w", payment.ErrMissingCurrency)
	}

	priceID := req.Meta["price_id"]
	if priceID == "" {
		return payment.Result{}, ErrMissingPriceID
	}

	create := &paddle.CreateTransactionRequest{
		Items: []paddle.CreateTransactionItems{
			*paddle.NewCreateTransactionItemsTransactionItemFromCatalog(&paddle.TransactionItemFromCatalog{
				PriceID:  priceID,
				Quantity: 1,
			}),
		},
		CurrencyCode: paddle.PtrTo(paddle.CurrencyCode(req.Currency)),
	}
	if cid := req.Meta["customer_id"]; cid != "" {
		create.CustomerID = paddle.PtrTo(cid)
	}

	if t := req.Meta["transit_id"]; t != "" {
		ctx = paddle.ContextWithTransitID(ctx, t)
	}

	txn, err := d.client.CreateTransaction(ctx, create)
	if err != nil {
		return payment.Result{}, fmt.Errorf("paddle: create: %w", err)
	}

	res, err := fromTransaction(txn)
	if err != nil {
		return payment.Result{}, err
	}

	if res.Amount != req.Amount {
		return payment.Result{}, fmt.Errorf("paddle: create: %w", payment.AmountMismatchError{Expected: req.Amount, Actual: res.Amount})
	}

	return res, nil
}

// GetPayment fetches a Paddle transaction by id.
func (d *driver) GetPayment(ctx context.Context, id string) (payment.Result, error) {
	if id == "" {
		return payment.Result{}, fmt.Errorf("paddle: get: %w", payment.ErrMissingPaymentID)
	}

	txn, err := d.client.GetTransaction(ctx, &paddle.GetTransactionRequest{TransactionID: id})
	if err != nil {
		return payment.Result{}, fmt.Errorf("paddle: get: %w", err)
	}

	return fromTransaction(txn)
}

func fromTransaction(txn *paddle.Transaction) (payment.Result, error) {
	var total int64

	if s := txn.Details.Totals.Total; s != "" {
		parsed, err := parsePaddleAmount(s)
		if err != nil {
			return payment.Result{}, fmt.Errorf("paddle: parse total: %w", err)
		}

		total = parsed
	}

	return payment.Result{
		ID:       txn.ID,
		Status:   payment.PaymentStatus(string(txn.Status)),
		Amount:   total,
		Currency: string(txn.CurrencyCode),
	}, nil
}

// parsePaddleAmount converts a Paddle decimal total to minor units, truncating
// toward zero without rounding (extra fractional digits are dropped).
func parsePaddleAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	if !strings.Contains(s, ".") {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, err
		}

		return v, nil
	}

	parts := strings.SplitN(s, ".", 2)
	intPartStr := parts[0]
	fracStr := parts[1]

	var intPart int64

	if intPartStr != "" && intPartStr != "-" {
		v, err := strconv.ParseInt(intPartStr, 10, 64)
		if err != nil {
			return 0, err
		}

		intPart = v
	}

	if len(fracStr) > 2 {
		fracStr = fracStr[:2]
	} else if len(fracStr) < 2 {
		fracStr += strings.Repeat("0", 2-len(fracStr))
	}

	fracVal, err := strconv.ParseInt(fracStr, 10, 64)
	if err != nil {
		return 0, err
	}

	if intPart < 0 || strings.HasPrefix(intPartStr, "-") {
		return intPart*100 - fracVal, nil
	}

	return intPart*100 + fracVal, nil
}

// Refund refunds amount (in minor units) against a Paddle transaction.
func (d *driver) Refund(ctx context.Context, id string, amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("paddle: refund: %w", payment.ErrInvalidAmount)
	}

	if id == "" {
		return fmt.Errorf("paddle: refund: %w", payment.ErrMissingPaymentID)
	}

	txn, err := d.client.GetTransaction(ctx, &paddle.GetTransactionRequest{TransactionID: id})
	if err != nil {
		return fmt.Errorf("paddle: refund: get transaction: %w", err)
	}

	total, err := parsePaddleAmount(txn.Details.Totals.Total)
	if err != nil {
		return fmt.Errorf("paddle: refund: parse total: %w", err)
	}

	if amount > total {
		return fmt.Errorf("paddle: refund: %w", payment.AmountMismatchError{Expected: total, Actual: amount})
	}

	req := &paddle.CreateAdjustmentRequest{
		Action:        paddle.AdjustmentActionRefund,
		Reason:        "refund",
		TransactionID: id,
	}

	switch {
	case amount == total:
		req.Type = paddle.PtrTo(paddle.AdjustmentTypeFull)
	case len(txn.Details.LineItems) == 1:
		itemTotal, err := parsePaddleAmount(txn.Details.LineItems[0].Totals.Total)
		if err != nil {
			return fmt.Errorf("paddle: refund: parse line item total: %w", err)
		}

		if amount > itemTotal {
			return fmt.Errorf("paddle: refund: %w", payment.AmountMismatchError{Expected: itemTotal, Actual: amount})
		}

		req.Type = paddle.PtrTo(paddle.AdjustmentTypePartial)
		amt := strconv.FormatInt(amount, 10)
		req.Items = []paddle.AdjustmentItemCreate{{
			ItemID: txn.Details.LineItems[0].ID,
			Type:   paddle.AdjustmentItemCreateTypePartial,
			Amount: &amt,
		}}
	default:
		return ErrMultiItemPartial
	}

	if _, err := d.client.CreateAdjustment(ctx, req); err != nil {
		return fmt.Errorf("paddle: refund: %w", err)
	}

	return nil
}

// WebhookEvent decodes and verifies a raw Paddle webhook payload.
func (d *driver) WebhookEvent(ctx context.Context, raw []byte, signature string) (payment.Event, error) {
	if len(raw) > d.maxWebhookBytes {
		return payment.Event{}, fmt.Errorf("paddle: webhook: %w", payment.SizeLimitError{Size: len(raw), Limit: d.maxWebhookBytes})
	}

	if d.webhookSecret == "" {
		return payment.Event{}, payment.ErrMissingWebhookSecret
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewReader(raw))
	if err != nil {
		return payment.Event{}, fmt.Errorf("paddle: webhook: %w", err)
	}

	req.Header.Set("Paddle-Signature", signature)

	ok, err := d.verifier.Verify(req)
	if err != nil || !ok {
		return payment.Event{}, payment.ErrInvalidSignature
	}

	var envelope struct {
		EventType string `json:"event_type"`
		Data      struct {
			ID           string `json:"id"`
			Status       string `json:"status"`
			CurrencyCode string `json:"currency_code"`
			Details      struct {
				Totals struct {
					Total string `json:"total"`
				} `json:"totals"`
			} `json:"details"`
		} `json:"data"`
	}

	if derr := payment.LimitDecode(raw, &envelope, d.maxWebhookBytes); derr != nil {
		return payment.Event{}, fmt.Errorf("paddle: webhook: unmarshal: %w", derr)
	}

	var amount int64
	if s := envelope.Data.Details.Totals.Total; s != "" {
		amount, err = parsePaddleAmount(s)
		if err != nil {
			return payment.Event{}, fmt.Errorf("paddle: webhook: parse total: %w", err)
		}
	}

	return payment.Event{
		Type: payment.EventType(envelope.EventType),
		Object: payment.Result{
			ID:       envelope.Data.ID,
			Status:   payment.PaymentStatus(envelope.Data.Status),
			Amount:   amount,
			Currency: envelope.Data.CurrencyCode,
		},
	}, nil
}

// Close releases backend resources.
func (d *driver) Close() error {
	return nil
}
