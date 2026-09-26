// Package ormdrill is the no-codegen orm reference drill: a hand-wired
// widget-shop model (widgets -> orders -> shipments, plus a widgets_archive
// copy and a promotions table) exercising the advanced query-builder surface
// end to end against in-memory SQLite, with no .zen schema and no codegen
// step. Tables, columns and relations are built with the exact constructors
// the zenorm backend would have generated (orm.NewTable / orm.NewColumn /
// orm.NewNullableColumn / orm.NewRelation) plus hand-written
// pointer-receiver Scan methods.
//
// Each topic is an exported Demo function printing one section; the
// per-topic Example tests assert that output, so the drill doubles as
// executable documentation under go test ./examples/ormdrill/. The
// cmd/app runner executes every demo in sequence against one database.
package ormdrill

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// Widgets is the hand-wired widgets table, mirroring zenorm codegen output.
var Widgets = orm.NewTable[Widget]("widgets", []string{"id", "name", "price_cents", "created_at", "note"})

// WidgetsArchive shares Widget's shape so it is a valid INSERT ... SELECT
// target/source (see DemoInsertSelectDistinct).
var WidgetsArchive = orm.NewTable[Widget]("widgets_archive", []string{"id", "name", "price_cents", "created_at", "note"})

// WidgetCols holds one typed column reference per Widget field.
var WidgetCols = struct {
	ID         orm.Column[Widget, string]
	Name       orm.Column[Widget, string]
	PriceCents orm.Column[Widget, int64]
	CreatedAt  orm.Column[Widget, time.Time]
	Note       orm.NullableColumn[Widget, string]
}{
	ID:         orm.NewColumn[Widget, string]("widgets", "id"),
	Name:       orm.NewColumn[Widget, string]("widgets", "name"),
	PriceCents: orm.NewColumn[Widget, int64]("widgets", "price_cents"),
	CreatedAt:  orm.NewColumn[Widget, time.Time]("widgets", "created_at"),
	Note:       orm.NewNullableColumn[Widget, string]("widgets", "note"),
}

// Widget is one shop widget. CreatedAt is stored as RFC3339Nano TEXT and parsed
// by Scan; Note is NULL for odd-numbered widgets.
type Widget struct {
	ID         string
	Name       string
	PriceCents int64
	CreatedAt  time.Time
	Note       orm.Option[string]
}

// Scan implements the orm row-scanner contract, reading columns positionally.
func (e *Widget) Scan(row orm.Row) error {
	var rawCreatedAt string

	if err := row.Scan(&e.ID, &e.Name, &e.PriceCents, &rawCreatedAt, &e.Note); err != nil {
		return err
	}

	t, err := time.Parse(time.RFC3339Nano, rawCreatedAt)
	if err != nil {
		return fmt.Errorf("parse widget created_at %q: %w", rawCreatedAt, err)
	}

	e.CreatedAt = t

	return nil
}

// Orders is the hand-wired orders table.
var Orders = orm.NewTable[Order]("orders", []string{"id", "widget_id", "amount_cents", "created_at"})

// OrderCols holds one typed column reference per Order field.
var OrderCols = struct {
	ID          orm.Column[Order, string]
	WidgetID    orm.Column[Order, string]
	AmountCents orm.Column[Order, int64]
	CreatedAt   orm.Column[Order, time.Time]
}{
	ID:          orm.NewColumn[Order, string]("orders", "id"),
	WidgetID:    orm.NewColumn[Order, string]("orders", "widget_id"),
	AmountCents: orm.NewColumn[Order, int64]("orders", "amount_cents"),
	CreatedAt:   orm.NewColumn[Order, time.Time]("orders", "created_at"),
}

// Order is one purchase of a widget.
type Order struct {
	ID          string
	WidgetID    string
	AmountCents int64
	CreatedAt   time.Time
}

// Scan implements the orm row-scanner contract, reading columns positionally.
func (e *Order) Scan(row orm.Row) error {
	var rawCreatedAt string

	if err := row.Scan(&e.ID, &e.WidgetID, &e.AmountCents, &rawCreatedAt); err != nil {
		return err
	}

	t, err := time.Parse(time.RFC3339Nano, rawCreatedAt)
	if err != nil {
		return fmt.Errorf("parse order created_at %q: %w", rawCreatedAt, err)
	}

	e.CreatedAt = t

	return nil
}

// Shipments is the hand-wired shipments table.
var Shipments = orm.NewTable[Shipment]("shipments", []string{"id", "order_id", "carrier", "tracking"})

// ShipmentCols holds one typed column reference per Shipment field.
var ShipmentCols = struct {
	ID       orm.Column[Shipment, string]
	OrderID  orm.Column[Shipment, string]
	Carrier  orm.Column[Shipment, string]
	Tracking orm.Column[Shipment, string]
}{
	ID:       orm.NewColumn[Shipment, string]("shipments", "id"),
	OrderID:  orm.NewColumn[Shipment, string]("shipments", "order_id"),
	Carrier:  orm.NewColumn[Shipment, string]("shipments", "carrier"),
	Tracking: orm.NewColumn[Shipment, string]("shipments", "tracking"),
}

// Shipment is one fulfilled order delivery.
type Shipment struct {
	ID       string
	OrderID  string
	Carrier  string
	Tracking string
}

// Scan implements the orm row-scanner contract, reading columns positionally.
func (e *Shipment) Scan(row orm.Row) error {
	return row.Scan(&e.ID, &e.OrderID, &e.Carrier, &e.Tracking)
}

// WidgetOrders is the one-to-many relation widgets.id = orders.widget_id.
// OrderShipments is orders.id = shipments.order_id.
var (
	// WidgetOrders relates each widget to its orders.
	WidgetOrders = orm.NewRelation[Widget, Order]("id", "widget_id", Orders)
	// OrderShipments relates each order to its shipments.
	OrderShipments = orm.NewRelation[Order, Shipment]("id", "order_id", Shipments)
)

// Promotions backs the partial-upsert-WHERE flow: a partial unique index
// (WHERE active = 1) gives the ON CONFLICT target predicate something real
// to name.
var Promotions = orm.NewTable[Promotion]("promotions", []string{"code", "discount_pct", "active"})

// PromotionCols holds one typed column reference per Promotion field.
var PromotionCols = struct {
	Code        orm.Column[Promotion, string]
	DiscountPct orm.Column[Promotion, int64]
	Active      orm.Column[Promotion, int64]
}{
	Code:        orm.NewColumn[Promotion, string]("promotions", "code"),
	DiscountPct: orm.NewColumn[Promotion, int64]("promotions", "discount_pct"),
	Active:      orm.NewColumn[Promotion, int64]("promotions", "active"),
}

// Promotion is one discount code; only active rows participate in the
// partial unique index.
type Promotion struct {
	Code        string
	DiscountPct int64
	Active      int64
}

// Scan implements the orm row-scanner contract, reading columns positionally.
func (e *Promotion) Scan(row orm.Row) error {
	return row.Scan(&e.Code, &e.DiscountPct, &e.Active)
}

// OpenShop opens a fresh in-memory SQLite database, creates the shop tables
// and seeds them. Each call returns an independent database, so Example
// tests never share mutable state.
func OpenShop(ctx context.Context) (db.DB, error) {
	//nolint:contextcheck // sqlite.New takes no ctx; OpenShop threads ctx through every Exec below.
	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		return nil, fmt.Errorf("open shop db: %w", err)
	}

	for _, stmt := range []string{
		`CREATE TABLE widgets (id text, name text, price_cents integer, created_at text, note text)`,
		`CREATE TABLE orders (id text, widget_id text, amount_cents integer, created_at text)`,
		`CREATE TABLE shipments (id text, order_id text, carrier text, tracking text)`,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			_ = conn.Close(ctx)

			return nil, fmt.Errorf("create shop table: %w", err)
		}
	}

	if err := SeedShop(ctx, conn); err != nil {
		_ = conn.Close(ctx)

		return nil, err
	}

	return conn, nil
}

// SeedShop inserts the drill fixture: 8 widgets, 15 orders spread across
// widgets w01..w05, and a shipment for every even-numbered order (odd orders
// are still in flight, so an INNER join over all three tables is a proper
// subset). It is idempotent only on a fresh database.
func SeedShop(ctx context.Context, conn db.DB) error {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	for i := 1; i <= 8; i++ {
		assignments := []orm.Assignment[Widget]{
			orm.Set(WidgetCols.ID, fmt.Sprintf("w%02d", i)),
			orm.Set(WidgetCols.Name, fmt.Sprintf("gadget-%02d", i)),
			orm.Set(WidgetCols.PriceCents, int64(500+50*i)),
			orm.Set(WidgetCols.CreatedAt, base.Add(time.Duration(i)*time.Minute)),
		}

		// Even widgets carry a note; odd widgets leave it NULL, which the
		// NULLS FIRST/LAST and COALESCE flows exercise.
		if i%2 == 0 {
			assignments = append(assignments, WidgetCols.Note.SetValue(fmt.Sprintf("batch-%02d", i)))
		}

		if err := orm.InsertInto(Widgets).Values(assignments...).Exec(ctx, conn); err != nil {
			return fmt.Errorf("insert widget w%02d: %w", i, err)
		}
	}

	// 15 orders; the amount repeats every 5 rows, so keyset pagination
	// orders by created_at (unique here) rather than the duplicated amount.
	for i := 1; i <= 15; i++ {
		insert := orm.InsertInto(Orders).Values(
			orm.Set(OrderCols.ID, fmt.Sprintf("o%02d", i)),
			orm.Set(OrderCols.WidgetID, fmt.Sprintf("w%02d", (i-1)%5+1)),
			orm.Set(OrderCols.AmountCents, int64(750+250*((i-1)%5))),
			orm.Set(OrderCols.CreatedAt, base.Add(time.Duration(i)*time.Hour)),
		)

		if err := insert.Exec(ctx, conn); err != nil {
			return fmt.Errorf("insert order o%02d: %w", i, err)
		}
	}

	for i := 2; i <= 15; i += 2 {
		insert := orm.InsertInto(Shipments).Values(
			orm.Set(ShipmentCols.ID, fmt.Sprintf("s%02d", i)),
			orm.Set(ShipmentCols.OrderID, fmt.Sprintf("o%02d", i)),
			orm.Set(ShipmentCols.Carrier, "hermes"),
			orm.Set(ShipmentCols.Tracking, fmt.Sprintf("H%03d", i)),
		)

		if err := insert.Exec(ctx, conn); err != nil {
			return fmt.Errorf("insert shipment s%02d: %w", i, err)
		}
	}

	return nil
}
