package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/queue"
)

type checkoutItem struct {
	ProductID string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
}

type checkoutRequest struct {
	Coupon   string         `json:"coupon"`
	GiftNote string         `json:"gift_note"`
	Items    []checkoutItem `json:"items"`
}

// handleCheckout creates a pending order with items, then enqueues the
// SendConfirmation job. Mirrors the schema's Checkout RPC shape
// (CheckoutRequest -> OrderReceipt).
func (a *API) handleCheckout(w http.ResponseWriter, req *http.Request) {
	var in checkoutRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(in.Items) == 0 {
		writeError(w, http.StatusBadRequest, "empty cart")
		return
	}

	ctx := req.Context()
	sub := subject(req)

	var total int64
	type line struct {
		productID string
		quantity  int32
		price     int64
	}
	lines := make([]line, 0, len(in.Items))
	for _, it := range in.Items {
		if it.Quantity < 1 {
			writeError(w, http.StatusBadRequest, "bad quantity")
			return
		}
		prows, err := a.DB.Query(ctx, `SELECT price_cents, stock FROM products WHERE id = ?`, it.ProductID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		var price, stock int64
		if prows.Next() {
			if err := prows.Scan(&price, &stock); err != nil {
				_ = prows.Close()
				writeError(w, http.StatusInternalServerError, "scan failed")
				return
			}
		} else {
			_ = prows.Close()
			writeError(w, http.StatusNotFound, "product not found")
			return
		}
		_ = prows.Close()
		if stock < int64(it.Quantity) {
			writeError(w, http.StatusConflict, "insufficient stock")
			return
		}
		total += price * int64(it.Quantity)
		lines = append(lines, line{productID: it.ProductID, quantity: it.Quantity, price: price})
	}

	orderID := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	note := strings.TrimSpace(in.GiftNote)
	if _, err := a.DB.Exec(ctx,
		`INSERT INTO orders (id, user_id, total_cents, status, priority, note, created_at) VALUES (?, ?, ?, 'pending', 'low', ?, ?)`,
		orderID, sub, total, nullIfEmpty(note), now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}
	for _, l := range lines {
		if _, err := a.DB.Exec(ctx,
			`INSERT INTO order_items (id, order_id, product_id, quantity, price_cents) VALUES (?, ?, ?, ?, ?)`,
			uuid.NewString(), orderID, l.productID, l.quantity, l.price,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		if _, err := a.DB.Exec(ctx, `UPDATE products SET stock = stock - ? WHERE id = ?`, l.quantity, l.productID); err != nil {
			writeError(w, http.StatusInternalServerError, "stock update failed")
			return
		}
	}

	if a.Queue != nil {
		body, _ := json.Marshal(map[string]string{"order_id": orderID})
		_ = a.Queue.Push(ctx, "orders.confirm", queue.NewPayload(body), queue.NewHeaders(nil))
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"order_id":    orderID,
		"total_cents": total,
		"message":     a.confirmedMessage(ctx, req, orderID, total),
		"version":     a.checkoutVersion(ctx),
	})
}

// handleListOrders returns the caller's orders, newest first.
func (a *API) handleListOrders(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	rows, err := a.DB.Query(ctx, `SELECT id, total_cents, status, priority, created_at FROM orders WHERE user_id = ? ORDER BY created_at DESC LIMIT 50`, subject(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	defer func() { _ = rows.Close() }()

	out := []map[string]any{}
	for rows.Next() {
		var id, status, priority, created string
		var total int64
		if err := rows.Scan(&id, &total, &status, &priority, &created); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "total_cents": total, "status": status, "priority": priority, "created_at": created})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetOrder returns one order owned by the caller.
func (a *API) handleGetOrder(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := req.PathValue("id")
	rows, err := a.DB.Query(ctx, `SELECT id, user_id, total_cents, status, priority, created_at FROM orders WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	var oid, owner, status, priority, created string
	var total int64
	if rows.Next() {
		if err := rows.Scan(&oid, &owner, &total, &status, &priority, &created); err != nil {
			_ = rows.Close()
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
	} else {
		_ = rows.Close()
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	_ = rows.Close()
	if owner != subject(req) {
		writeError(w, http.StatusForbidden, "no access")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": oid, "total_cents": total, "status": status, "priority": priority, "created_at": created})
}

type reviewRequest struct {
	ProductID string `json:"product_id"`
	Rating    int32  `json:"rating"`
	Body      string `json:"body"`
}

// handleCreateReview stores a 1-5 rating for a product.
func (a *API) handleCreateReview(w http.ResponseWriter, req *http.Request) {
	var in reviewRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if in.Rating < 1 || in.Rating > 5 {
		writeError(w, http.StatusBadRequest, "rating 1-5")
		return
	}
	if strings.TrimSpace(in.Body) == "" {
		writeError(w, http.StatusBadRequest, "body required")
		return
	}

	ctx := req.Context()
	prows, err := a.DB.Query(ctx, `SELECT id FROM products WHERE id = ?`, in.ProductID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	found := prows.Next()
	_ = prows.Close()
	if !found {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := a.DB.Exec(ctx,
		`INSERT INTO reviews (id, product_id, user_id, rating, body, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, in.ProductID, subject(req), in.Rating, strings.TrimSpace(in.Body), now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// handleListReviews lists reviews, optionally filtered by ?product_id=.
func (a *API) handleListReviews(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	productID := req.URL.Query().Get("product_id")
	const baseQuery = `SELECT id, product_id, user_id, rating, body, created_at FROM reviews ORDER BY created_at DESC LIMIT 50`
	const filteredQuery = `SELECT id, product_id, user_id, rating, body, created_at FROM reviews WHERE product_id = ? ORDER BY created_at DESC LIMIT 50`
	var rows db.Rows
	var err error
	if productID != "" {
		rows, err = a.DB.Query(ctx, filteredQuery, productID)
	} else {
		rows, err = a.DB.Query(ctx, baseQuery)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	defer func() { _ = rows.Close() }()

	out := []map[string]any{}
	for rows.Next() {
		var id, pid, uid, body, created string
		var rating int32
		if err := rows.Scan(&id, &pid, &uid, &rating, &body, &created); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "product_id": pid, "user_id": uid, "rating": rating, "body": body, "created_at": created})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
