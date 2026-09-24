package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/billing"
	"github.com/zenta-dev/zever/idempotency"
	"github.com/zenta-dev/zever/payment"
	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/queue"
	"github.com/zenta-dev/zever/router"
)

// Booking is the JSON shape for bookings.
type Booking struct {
	ID        string `json:"id"`
	SpaceID   string `json:"space_id"`
	GuestID   string `json:"guest_id"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type createBookingRequest struct {
	SpaceID        string `json:"space_id"`
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date"`
	GuestPhone     string `json:"guest_phone"`
	IdempotencyKey string `json:"idempotency_key"`
}

type bookingResponse struct {
	Booking         Booking `json:"booking"`
	Message         string  `json:"message"`
	CheckoutVersion string  `json:"checkout_version"`
	Tenant          string  `json:"tenant"`
	PaymentID       string  `json:"payment_id"`
}

// bookingKey resolves the idempotency key for a booking attempt.
func bookingKey(req *http.Request, in createBookingRequest, guest string) string {
	if k := strings.TrimSpace(req.Header.Get("Idempotency-Key")); k != "" {
		return "booking:" + k
	}
	if k := strings.TrimSpace(in.IdempotencyKey); k != "" {
		return "booking:" + k
	}
	return "booking:" + guest + ":" + in.SpaceID + ":" + in.StartDate + ":" + in.EndDate
}

// handleCreateBooking runs the guarded book flow: ratelimit, idempotency,
// date lock, overlap check, stub charge, host subscription, queue dispatch,
// encrypted phone, tenant resolve, i18n message, flag-gated checkout.
func (a *API) handleCreateBooking(w http.ResponseWriter, req *http.Request) {
	var in createBookingRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(in.SpaceID) == "" || strings.TrimSpace(in.StartDate) == "" || strings.TrimSpace(in.EndDate) == "" {
		writeError(w, http.StatusBadRequest, "space_id, start_date, end_date required")
		return
	}
	if in.StartDate >= in.EndDate {
		writeError(w, http.StatusBadRequest, "end_date must be after start_date")
		return
	}

	ctx := req.Context()
	guest := subject(req)
	tenant := a.tenantID(ctx, req)

	if a.RateLimit != nil {
		dec, err := a.RateLimit.Allow(ctx, "book:"+guest, 1)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ratelimit failed")
			return
		}
		if !dec.Allowed {
			writeError(w, http.StatusTooManyRequests, "rate limited")
			return
		}
	}

	key := bookingKey(req, in, guest)
	fingerprint := []byte(guest + "|" + in.SpaceID + "|" + in.StartDate + "|" + in.EndDate)
	if a.Idempotency != nil {
		outcome, err := a.Idempotency.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fingerprint})
		if err != nil {
			if errors.Is(err, idempotency.ErrInProgress) {
				writeError(w, http.StatusConflict, "booking in progress")
				return
			}
			if errors.Is(err, idempotency.ErrKeyMismatch) {
				writeError(w, http.StatusBadRequest, "idempotency key mismatch")
				return
			}
			writeError(w, http.StatusInternalServerError, "idempotency failed")
			return
		}
		if outcome.Replay && len(outcome.Result) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(outcome.Result)
			return
		}
	}

	completeIdem := func(payload []byte) {
		if a.Idempotency == nil {
			return
		}
		_ = a.Idempotency.Complete(ctx, key, fingerprint, payload)
	}

	lockKey := "booking:" + in.SpaceID + ":" + in.StartDate + ":" + in.EndDate
	if a.Lock != nil {
		lk, ok, err := a.Lock.TryAcquire(ctx, lockKey, 30*time.Second)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lock failed")
			return
		}
		if !ok {
			writeError(w, http.StatusConflict, "dates already booked")
			return
		}
		defer func() { _ = lk.Unlock(ctx) }()
	}

	// Space must exist.
	srows, err := a.DB.Query(ctx, `SELECT id, host_id, title, price_cents FROM spaces WHERE id = ?`, in.SpaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "space lookup failed")
		return
	}
	var spaceID, hostID, spaceTitle string
	var priceCents int64
	hasSpace := false
	if srows.Next() {
		if serr := srows.Scan(&spaceID, &hostID, &spaceTitle, &priceCents); serr == nil {
			hasSpace = true
		}
	}
	_ = srows.Close()
	if serr := srows.Err(); serr != nil {
		writeError(w, http.StatusInternalServerError, "space lookup failed")
		return
	}
	if !hasSpace {
		writeError(w, http.StatusNotFound, "space not found")
		return
	}

	// Overlap check against confirmed bookings.
	orows, err := a.DB.Query(ctx,
		`SELECT id FROM bookings WHERE space_id = ? AND status != 'cancelled' AND NOT (end_date <= ? OR start_date >= ?) LIMIT 1`,
		in.SpaceID, in.StartDate, in.EndDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "availability check failed")
		return
	}
	overlap := orows.Next()
	_ = orows.Close()
	if err := orows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "availability check failed")
		return
	}
	if overlap {
		writeError(w, http.StatusConflict, "dates already booked")
		return
	}

	// Stub charge for the stay.
	var payResult payment.Result
	if a.Payment != nil {
		pr, err := a.Payment.CreatePayment(ctx, payment.Request{
			Amount: priceCents, Currency: "USD", Method: payment.MethodCard,
			Meta: map[string]string{"space_id": in.SpaceID, "guest_id": guest},
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "charge failed")
			return
		}
		payResult = pr
	} else {
		payResult = payment.Result{ID: "pay_" + uuid.NewString(), Status: payment.PaymentSucceeded, Amount: priceCents, Currency: "USD"}
	}

	// Host subscription via billing stub (best-effort, still exercised).
	if a.Billing != nil {
		hrows, herr := a.DB.Query(ctx, `SELECT email FROM users WHERE id = ?`, hostID)
		hostEmail := ""
		if herr == nil {
			if hrows.Next() {
				_ = hrows.Scan(&hostEmail)
			}
			_ = hrows.Close()
		}
		cust, cerr := a.Billing.CreateCustomer(ctx, "host "+hostID, hostEmail, "billing:"+hostID+":"+in.SpaceID)
		if cerr == nil {
			_, _ = a.Billing.CreateSubscription(ctx, cust.ID, "host-plan", "sub:"+cust.ID+":"+in.SpaceID)
		} else {
			_, _ = a.Billing.CreateSubscription(ctx, "cust_"+hostID, "host-plan", "sub:"+hostID+":"+in.SpaceID)
		}
		_ = billing.SubscriptionActive
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := a.DB.Exec(ctx,
		`INSERT INTO bookings (id, space_id, guest_id, start_date, end_date, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, in.SpaceID, guest, in.StartDate, in.EndDate, "confirmed", now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "booking failed")
		return
	}

	if err := EnsureExtraTables(ctx, a.DB); err != nil {
		writeError(w, http.StatusInternalServerError, "booking failed")
		return
	}
	if _, err := a.DB.Exec(ctx,
		`INSERT INTO booking_payments (booking_id, payment_id, amount_cents) VALUES (?, ?, ?)`,
		id, payResult.ID, payResult.Amount); err != nil {
		writeError(w, http.StatusInternalServerError, "booking failed")
		return
	}
	if strings.TrimSpace(in.GuestPhone) != "" && a.Crypto != nil {
		cipher, err := a.Crypto.Encrypt(ctx, []byte(strings.TrimSpace(in.GuestPhone)))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "phone encrypt failed")
			return
		}
		if _, err := a.DB.Exec(ctx, `INSERT INTO booking_phones (booking_id, cipher) VALUES (?, ?)`, id, cipher); err != nil {
			writeError(w, http.StatusInternalServerError, "phone store failed")
			return
		}
	}

	// Queue confirmation dispatch.
	if a.Queue != nil {
		body, _ := json.Marshal(map[string]string{"booking_id": id, "space_id": in.SpaceID, "guest_id": guest})
		if err := a.Queue.Push(ctx, "bookings.confirm", queue.NewPayload(body), queue.NewHeaders(map[string]string{"tenant": tenant})); err != nil {
			writeError(w, http.StatusInternalServerError, "confirmation dispatch failed")
			return
		}
	}

	resp := bookingResponse{
		Booking:         Booking{ID: id, SpaceID: in.SpaceID, GuestID: guest, StartDate: in.StartDate, EndDate: in.EndDate, Status: "confirmed", CreatedAt: now},
		Message:         a.confirmedMessage(ctx, req, spaceTitle),
		CheckoutVersion: a.checkoutVersion(ctx),
		Tenant:          tenant,
		PaymentID:       payResult.ID,
	}
	raw, _ := json.Marshal(resp)
	completeIdem(raw)
	writeJSON(w, http.StatusCreated, resp)
}

// handleListBookings lists the caller's bookings.
func (a *API) handleListBookings(w http.ResponseWriter, req *http.Request) {
	rows, err := a.DB.Query(req.Context(),
		`SELECT id, space_id, guest_id, start_date, end_date, status, created_at FROM bookings WHERE guest_id = ? ORDER BY created_at ASC`,
		subject(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	defer func() { _ = rows.Close }()
	out := []Booking{}
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.SpaceID, &b.GuestID, &b.StartDate, &b.EndDate, &b.Status, &b.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCancelBooking cancels a booking and refunds the stub charge.
// Only the guest owner may cancel (booking.cancel owner rule).
func (a *API) handleCancelBooking(w http.ResponseWriter, req *http.Request) {
	id := router.Param(req, "id")
	ctx := req.Context()
	sub := subject(req)

	rows, err := a.DB.Query(ctx,
		`SELECT id, space_id, guest_id, start_date, end_date, status, created_at FROM bookings WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	var b Booking
	found := false
	if rows.Next() {
		if serr := rows.Scan(&b.ID, &b.SpaceID, &b.GuestID, &b.StartDate, &b.EndDate, &b.Status, &b.CreatedAt); serr == nil {
			found = true
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	if a.Permission != nil {
		dec, err := a.Permission.Can(ctx,
			permission.Subject{ID: sub, Roles: []string{"user"}},
			"booking.cancel",
			permission.Resource{Type: "Booking", ID: id, Attributes: map[string]string{"owner": b.GuestID}})
		if err != nil || !dec.Allowed {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	} else if b.GuestID != sub {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if b.Status == "cancelled" {
		writeJSON(w, http.StatusOK, b)
		return
	}

	// Refund the linked stub payment (best-effort lookup).
	if a.Payment != nil {
		prows, err := a.DB.Query(ctx, `SELECT payment_id, amount_cents FROM booking_payments WHERE booking_id = ?`, id)
		if err == nil {
			var pid string
			var amt int64
			if prows.Next() {
				if serr := prows.Scan(&pid, &amt); serr == nil && pid != "" && amt > 0 {
					_ = a.Payment.Refund(ctx, pid, amt)
				}
			}
			_ = prows.Close()
		}
	}

	if _, err := a.DB.Exec(ctx, `UPDATE bookings SET status = 'cancelled' WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "cancel failed")
		return
	}
	b.Status = "cancelled"

	if a.Queue != nil {
		body, _ := json.Marshal(map[string]string{"booking_id": id, "status": "cancelled"})
		_ = a.Queue.Push(ctx, "bookings.cancel", queue.NewPayload(body), queue.NewHeaders(nil))
	}

	writeJSON(w, http.StatusOK, b)
}
