package api

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/db"
	gen "github.com/zenta-dev/zever/examples/demoapp/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/router"
)

// listOrders returns the caller's orders, newest last.
func (s *Server) listOrders(w http.ResponseWriter, req *http.Request) {
	rows, err := orm.From(gen.Orders).
		Where(gen.OrderCols.UserID.Eq(subject(req))).
		OrderBy(gen.OrderCols.CreatedAt.Asc()).
		Limit(100).
		All(req.Context(), s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": rows})
}

// getOrder returns one of the caller's orders and demos the lock battery.
func (s *Server) getOrder(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := router.Param(req, "id")
	row, found, err := orm.From(gen.Orders).
		Where(gen.OrderCols.ID.Eq(id)).
		First(ctx, s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if !found || row.UserID != subject(req) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if s.Lock != nil {
		if lk, ok, _ := s.Lock.TryAcquire(ctx, "order:"+id, 5*time.Second); ok && lk != nil {
			defer func() { _ = lk.Unlock(ctx) }()
		}
	}
	writeJSON(w, http.StatusOK, row)
}

type createOrderRequest struct {
	ProductIDs []string `json:"product_ids"`
}

// createOrder totals the given products into a pending order with one
// OrderItem per product, then dispatches ProcessOrder and publishes
// order.created as best-effort side effects.
func (s *Server) createOrder(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	sub := subject(req)

	var body createOrderRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(body.ProductIDs) == 0 {
		writeError(w, http.StatusBadRequest, "product_ids required")
		return
	}

	var total int64
	type line struct {
		id    string
		price int64
	}
	lines := make([]line, 0, len(body.ProductIDs))
	for _, pid := range body.ProductIDs {
		p, found, err := orm.From(gen.Products).Where(gen.ProductCols.ID.Eq(pid)).First(ctx, s.DB)
		if err != nil || !found {
			writeError(w, http.StatusBadRequest, "unknown product")
			return
		}
		total += p.PriceCents
		lines = append(lines, line{id: pid, price: p.PriceCents})
	}

	order := &gen.Order{
		ID:         uuid.NewString(),
		UserID:     sub,
		TotalCents: total,
		Status:     "pending",
		CreatedAt:  time.Now().UTC(),
	}
	// The order and every one of its line items commit as one atomic unit:
	// a mid-loop insert failure must never leave an order with missing line
	// items visible to anyone.
	err := db.WithTx(ctx, s.DB, nil, func(txCtx context.Context, tx db.Tx) error {
		if err := orm.InsertInto(gen.Orders).Values(
			orm.Set(gen.OrderCols.ID, order.ID),
			orm.Set(gen.OrderCols.UserID, order.UserID),
			orm.Set(gen.OrderCols.TotalCents, order.TotalCents),
			orm.Set(gen.OrderCols.Status, order.Status),
			orm.Set(gen.OrderCols.CreatedAt, order.CreatedAt),
		).Exec(txCtx, tx); err != nil {
			return err
		}

		for _, l := range lines {
			if err := orm.InsertInto(gen.OrderItems).Values(
				orm.Set(gen.OrderItemCols.ID, uuid.NewString()),
				orm.Set(gen.OrderItemCols.OrderID, order.ID),
				orm.Set(gen.OrderItemCols.ProductID, l.id),
				orm.Set(gen.OrderItemCols.Quantity, int64(1)),
				orm.Set(gen.OrderItemCols.PriceCents, l.price),
			).Exec(txCtx, tx); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create order")
		return
	}

	if s.Job != nil {
		_ = s.Job.Dispatch(ctx, "ProcessOrder", map[string]any{"order_id": order.ID})
	}
	if s.EventBus != nil {
		_ = s.EventBus.Publish(ctx, "order.created", []byte(order.ID), nil)
	}
	if s.Session != nil {
		sess, err := s.Session.Create(ctx, time.Hour)
		if err == nil {
			sess.Data = map[string]any{"last_order": order.ID}
			_ = s.Session.Save(ctx, sess)
		}
	}

	writeJSON(w, http.StatusCreated, order)
}
