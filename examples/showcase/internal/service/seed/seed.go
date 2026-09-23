// Package seed populates the showcase database with development data.
package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/db"
	gen "github.com/zenta-dev/zever/examples/showcase/generated/zenorm/orm/gen/shop"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/password"

	// Blank import registers the argon2id adapter used to hash demo passwords.
	_ "github.com/zenta-dev/zever/password/argon2"
)

// Demo identities and content. IDs are fixed so re-runs find existing rows.
const (
	AdminEmail   = "admin@example.com"
	ShopperEmail = "shopper@example.com"
	AdminID      = "seed-admin"
	ShopperID    = "seed-shopper"
	CategoryID   = "seed-category-gadgets"
	ProductOneID = "seed-product-widget"
	ProductTwoID = "seed-product-gizmo"
	TagID        = "seed-tag-featured"
	OrderID      = "seed-order-one"
	DemoPassword = "demo-pass-12"
)

// Run seeds the database: one admin, one shopper, one category, two
// products, one tag linked to both products, and one pending order.
// Seeding is idempotent: existing rows are left untouched.
func Run(ctx context.Context, database db.DB) error {
	if err := ensureUser(ctx, database, AdminID, AdminEmail, "Ada Admin", gen.RoleAdmin); err != nil {
		return err
	}
	if err := ensureUser(ctx, database, ShopperID, ShopperEmail, "Sam Shopper", gen.RoleMember); err != nil {
		return err
	}
	if err := ensureCategory(ctx, database); err != nil {
		return err
	}
	if err := ensureProduct(ctx, database, ProductOneID, "WIDGET-001", "Widget", 2500, 100); err != nil {
		return err
	}
	if err := ensureProduct(ctx, database, ProductTwoID, "GIZMO-002", "Gizmo", 4999, 50); err != nil {
		return err
	}
	if err := EnsureJoinTable(ctx, database); err != nil {
		return err
	}
	if err := ensureTag(ctx, database); err != nil {
		return err
	}
	return ensureOrder(ctx, database)
}

func ensureUser(ctx context.Context, database db.DB, id, email, name string, role gen.Role) error {
	existing, ok, err := orm.From(gen.Users).Where(gen.UserCols.Email.Eq(email)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup user: %w", err)
	}
	if ok {
		_ = existing
		return nil
	}

	hash, err := password.Hash(ctx, DemoPassword)
	if err != nil {
		return fmt.Errorf("[seed] hash password: %w", err)
	}

	if err := orm.InsertInto(gen.Users).Values(
		orm.Set(gen.UserCols.ID, id),
		orm.Set(gen.UserCols.Email, email),
		orm.Set(gen.UserCols.Name, name),
		orm.Set(gen.UserCols.Role, role),
		orm.Set(gen.UserCols.PasswordHash, hash),
		orm.Set(gen.UserCols.Age, int32(30)),
		orm.Set(gen.UserCols.CreditCents, int64(0)),
		orm.Set(gen.UserCols.Rating, float32(4.5)),
		orm.Set(gen.UserCols.Score, float64(9.75)),
		orm.Set(gen.UserCols.Verified, true),
		orm.Set(gen.UserCols.Birthday, time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)),
		orm.Set(gen.UserCols.Avatar, []byte{0x89, 0x50}),
		orm.Set(gen.UserCols.Prefs, orm.JSONText(`{"theme":"dark"}`)),
		orm.Set(gen.UserCols.CreatedAt, time.Now().UTC()),
	).Exec(ctx, database); err != nil {
		return fmt.Errorf("[seed] insert user: %w", err)
	}
	return nil
}

func ensureCategory(ctx context.Context, database db.DB) error {
	_, ok, err := orm.From(gen.Categories).Where(gen.CategoryCols.ID.Eq(CategoryID)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup category: %w", err)
	}
	if ok {
		return nil
	}

	return insertErr(orm.InsertInto(gen.Categories).Values(
		orm.Set(gen.CategoryCols.ID, CategoryID),
		orm.Set(gen.CategoryCols.Name, "Gadgets"),
		orm.Set(gen.CategoryCols.Description, "Demo gadgets"),
		orm.Set(gen.CategoryCols.CreatedAt, time.Now().UTC()),
	).Exec(ctx, database), "category")
}

func ensureProduct(ctx context.Context, database db.DB, id, sku, headline string, price, stock int64) error {
	_, ok, err := orm.From(gen.Products).Where(gen.ProductCols.ID.Eq(id)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup product: %w", err)
	}
	if ok {
		return nil
	}

	return insertErr(orm.InsertInto(gen.Products).Values(
		orm.Set(gen.ProductCols.ID, id),
		orm.Set(gen.ProductCols.CategoryID, CategoryID),
		orm.Set(gen.ProductCols.Sku, sku),
		orm.Set(gen.ProductCols.Headline, headline),
		orm.Set(gen.ProductCols.Description, "Seeded demo product"),
		orm.Set(gen.ProductCols.PriceCents, price),
		orm.Set(gen.ProductCols.Stock, stock),
		orm.Set(gen.ProductCols.Weight, 1.5),
		orm.Set(gen.ProductCols.Featured, true),
		orm.Set(gen.ProductCols.CreatedAt, time.Now().UTC()),
	).Exec(ctx, database), "product")
}

// ensureTag inserts the featured tag and links it to both demo products
// through the many_to_many join table (no entity of its own, so raw SQL).
func ensureTag(ctx context.Context, database db.DB) error {
	_, ok, err := orm.From(gen.Tags).Where(gen.TagCols.ID.Eq(TagID)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup tag: %w", err)
	}
	if !ok {
		if err := insertErr(orm.InsertInto(gen.Tags).Values(
			orm.Set(gen.TagCols.ID, TagID),
			orm.Set(gen.TagCols.Name, "featured"),
			orm.Set(gen.TagCols.CreatedAt, time.Now().UTC()),
		).Exec(ctx, database), "tag"); err != nil {
			return err
		}
	}

	for _, pid := range []string{ProductOneID, ProductTwoID} {
		if _, err := database.Exec(ctx,
			`INSERT OR IGNORE INTO product_tags (product_id, tag_id) VALUES (?, ?)`, pid, TagID); err != nil {
			return fmt.Errorf("[seed] link tag: %w", err)
		}
	}
	return nil
}

func ensureOrder(ctx context.Context, database db.DB) error {
	_, ok, err := orm.From(gen.Orders).Where(gen.OrderCols.ID.Eq(OrderID)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup order: %w", err)
	}
	if ok {
		return nil
	}

	now := time.Now().UTC()
	if err := insertErr(orm.InsertInto(gen.Orders).Values(
		orm.Set(gen.OrderCols.ID, OrderID),
		orm.Set(gen.OrderCols.UserID, ShopperID),
		orm.Set(gen.OrderCols.TotalCents, int64(7499)),
		orm.Set(gen.OrderCols.Status, gen.OrderStatusPending),
		orm.Set(gen.OrderCols.Priority, "high"),
		gen.OrderCols.Note.SetValue("seeded order"),
		orm.Set(gen.OrderCols.CreatedAt, now),
	).Exec(ctx, database), "order"); err != nil {
		return err
	}

	return insertErr(orm.InsertInto(gen.OrderItems).Values(
		orm.Set(gen.OrderItemCols.ID, "seed-order-item-one"),
		orm.Set(gen.OrderItemCols.OrderID, OrderID),
		orm.Set(gen.OrderItemCols.ProductID, ProductOneID),
		orm.Set(gen.OrderItemCols.Quantity, int32(3)),
		orm.Set(gen.OrderItemCols.PriceCents, int64(2500)),
	).Exec(ctx, database), "order item")
}

func insertErr(err error, what string) error {
	if err != nil {
		return fmt.Errorf("[seed] insert %s: %w", what, err)
	}
	return nil
}

// EnsureJoinTable creates the product_tags join table for the
// Product<->Tag many_to_many relation. The atlas backend materializes
// entity tables only, so the join table is hand-owned (same pattern as
// bookings' EnsureExtraTables).
func EnsureJoinTable(ctx context.Context, database db.DB) error {
	_, err := database.Exec(ctx, `CREATE TABLE IF NOT EXISTS product_tags (product_id TEXT NOT NULL REFERENCES products (id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags (id) ON DELETE CASCADE, PRIMARY KEY (product_id, tag_id))`)
	return err
}
