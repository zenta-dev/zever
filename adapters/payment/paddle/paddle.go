package paddle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PaddleHQ/paddle-go-sdk/v5"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/shared/httpclient"
	"github.com/zenta-dev/zever/shared/providersopt"
)

// newSDK constructs the Paddle SDK. It is a variable so tests can stub construction.
var newSDK = paddle.New

// DefaultTimestampTolerance bounds webhook timestamp skew accepted by the verifier.
const DefaultTimestampTolerance = 5 * time.Minute

// driver is a payment.Payment backed by Paddle transactions and adjustments.
type driver struct {
	client          *paddle.SDK
	verifier        *paddle.WebhookVerifier
	webhookSecret   string
	maxWebhookBytes int
	idempotency     idempotency.Store
}

// refundFingerprint scopes an idempotency key to a specific refund request.
func refundFingerprint(id string, amount int64) []byte {
	sum := sha256.Sum256([]byte(id + "|" + strconv.FormatInt(amount, 10)))

	return sum[:]
}

// beginRefundGuard reserves key in store; ok=false means replay, skip execution.
func beginRefundGuard(ctx context.Context, store idempotency.Store, key string, fingerprint []byte) (ok bool, err error) {
	if store == nil || key == "" {
		return true, nil
	}

	outcome, err := store.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fingerprint, TTL: payment.DefaultRefundIdempotencyTTL})
	if err != nil {
		return false, err
	}

	if outcome.Replay {
		return false, nil
	}

	return true, nil
}

// New creates a Payment backed by Paddle.
func New(o payment.Options) (payment.Payment, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("paddle: %w", err)
	}

	if o.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	if o.WebhookSecret == "" {
		return nil, payment.ErrMissingWebhookSecret
	}

	endpoint := providersopt.PaddleEndpoint(o.Endpoint, o.Sandbox)

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
			o.WebhookSecret, paddle.VerifierWithTimestampTolerance(DefaultTimestampTolerance),
		),
		webhookSecret:   o.WebhookSecret,
		maxWebhookBytes: maxWebhookBytes,
		idempotency:     o.Idempotency,
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

// minorUnitsPerMajor scales a Paddle whole-unit total to minor units.
const minorUnitsPerMajor = 100

// parsePaddleAmount converts a Paddle decimal total (major units) to minor
// units. Whole-unit totals scale by minorUnitsPerMajor. Sub-cent precision
// rounds half away from zero so positive and negative totals stay symmetric.
func parsePaddleAmount(s string) (int64, error) {
	orig := s
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	if !strings.Contains(s, ".") {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			if errors.Is(err, strconv.ErrRange) {
				return 0, ErrAmountOverflow
			}

			return 0, err
		}

		if v > math.MaxInt64/minorUnitsPerMajor || v < math.MinInt64/minorUnitsPerMajor {
			return 0, ErrAmountOverflow
		}

		return v * minorUnitsPerMajor, nil
	}

	parts := strings.SplitN(s, ".", 2)
	intPartStr := parts[0]
	fracStr := parts[1]

	neg := false
	magStr := intPartStr
	if strings.HasPrefix(magStr, "-") || strings.HasPrefix(magStr, "+") {
		neg = magStr[0] == '-'
		magStr = magStr[1:]
	}

	for i := 0; i < len(magStr); i++ {
		if magStr[i] < '0' || magStr[i] > '9' {
			return 0, fmt.Errorf("paddle: invalid amount %q", orig)
		}
	}

	for i := 0; i < len(fracStr); i++ {
		if fracStr[i] < '0' || fracStr[i] > '9' {
			return 0, fmt.Errorf("paddle: invalid amount %q", orig)
		}
	}

	mag := uint64(0)
	if magStr != "" {
		v, err := strconv.ParseUint(magStr, 10, 64)
		if err != nil {
			return 0, err
		}

		mag = v
	}

	var base uint64

	var roundUp bool

	switch {
	case len(fracStr) == 0:
		base = 0
	case len(fracStr) == 1:
		base = uint64(fracStr[0]-'0') * 10
	case len(fracStr) == 2:
		base = uint64(fracStr[0]-'0')*10 + uint64(fracStr[1]-'0')
	default:
		base = uint64(fracStr[0]-'0')*10 + uint64(fracStr[1]-'0')
		roundUp = fracStr[2] >= '5'
	}

	limit := uint64(math.MaxInt64)
	if neg {
		limit = uint64(math.MaxInt64) + 1
	}

	if mag > limit/uint64(minorUnitsPerMajor) {
		return 0, ErrAmountOverflow
	}

	cents := mag*uint64(minorUnitsPerMajor) + base
	if cents > limit {
		return 0, ErrAmountOverflow
	}

	if roundUp {
		if cents == limit {
			return 0, ErrAmountOverflow
		}

		cents++
	}

	if neg {
		if cents == uint64(math.MaxInt64)+1 {
			return math.MinInt64, nil
		}

		return -int64(cents), nil //nolint:gosec // G115: cents <= limit <= MaxInt64+1, MinInt64 handled above
	}

	return int64(cents), nil //nolint:gosec // G115: cents <= limit == MaxInt64 here
}

// formatPaddleAmount renders minor units as a Paddle major-unit decimal so
// outgoing partial-refund adjustments agree with parsed totals.
func formatPaddleAmount(amount int64) string {
	neg := amount < 0

	var mag uint64
	if neg {
		mag = uint64(-(amount + 1)) + 1 //nolint:gosec // G115: amount < 0 so -(amount+1) >= 0 fits
	} else {
		mag = uint64(amount)
	}

	sign := ""
	if neg {
		sign = "-"
	}

	return fmt.Sprintf("%s%d.%02d", sign, mag/uint64(minorUnitsPerMajor), mag%uint64(minorUnitsPerMajor))
}

// Refund refunds amount (in minor units) against a Paddle transaction.
// key is the idempotency key; empty skips the idempotency guard.
func (d *driver) Refund(ctx context.Context, id string, amount int64, key string) error {
	if amount <= 0 {
		return fmt.Errorf("paddle: refund: %w", payment.ErrInvalidAmount)
	}

	if id == "" {
		return fmt.Errorf("paddle: refund: %w", payment.ErrMissingPaymentID)
	}

	fingerprint := refundFingerprint(id, amount)

	execute, err := beginRefundGuard(ctx, d.idempotency, key, fingerprint)
	if err != nil {
		return fmt.Errorf("paddle: refund: %w", err)
	}

	if !execute {
		return nil
	}

	if err := d.refund(ctx, id, amount); err != nil {
		if d.idempotency != nil && key != "" {
			_ = d.idempotency.Forget(ctx, key)
		}

		return err
	}

	if d.idempotency != nil && key != "" {
		if err := d.idempotency.Complete(ctx, key, fingerprint, []byte{1}); err != nil {
			return fmt.Errorf("paddle: refund: %w", err)
		}
	}

	return nil
}

func (d *driver) refund(ctx context.Context, id string, amount int64) error {
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
		amt := formatPaddleAmount(amount)
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

// webhookFingerprint binds a dedupe record to the exact payload delivered.
func webhookFingerprint(raw []byte) []byte {
	sum := sha256.Sum256(raw)

	return sum[:]
}

// dedupeWebhookEvent reserves event id in store and marks it seen. A second
// delivery of the same id returns DuplicateEventError; a delivery with the
// same id but a different payload trips the store's fingerprint mismatch.
func dedupeWebhookEvent(ctx context.Context, store idempotency.Store, key, eventID string, fingerprint []byte) error {
	if store == nil || eventID == "" {
		return nil
	}

	outcome, err := store.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fingerprint, TTL: payment.DefaultWebhookIdempotencyTTL})
	if err != nil {
		return err
	}

	if outcome.Replay {
		return payment.DuplicateEventError{EventID: eventID}
	}

	if err := store.Complete(ctx, key, fingerprint, []byte{1}); err != nil {
		_ = store.Forget(ctx, key)

		return err
	}

	return nil
}

// WebhookEvent decodes and verifies a raw Paddle webhook payload.
// When an idempotency store is configured, the provider event id is
// reserved before returning: a repeat delivery fails with
// payment.ErrDuplicateEvent instead of re-executing downstream work.
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
		EventID   string `json:"event_id"`
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

	ev := payment.Event{
		ID:   envelope.EventID,
		Type: payment.EventType(envelope.EventType),
		Object: payment.Result{
			ID:       envelope.Data.ID,
			Status:   payment.PaymentStatus(envelope.Data.Status),
			Amount:   amount,
			Currency: envelope.Data.CurrencyCode,
		},
	}

	if err := dedupeWebhookEvent(ctx, d.idempotency, "paddle:webhook:"+envelope.EventID, envelope.EventID, webhookFingerprint(raw)); err != nil {
		return payment.Event{}, fmt.Errorf("paddle: webhook: %w", err)
	}

	return ev, nil
}

// Close releases backend resources.
func (d *driver) Close() error {
	return nil
}
