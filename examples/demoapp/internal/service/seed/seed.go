// Package seed populates the demo database with development data.
package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/password"
	gen "github.com/zenta-dev/zever/examples/demoapp/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"

	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
)

// Demo identities and content. IDs are fixed so re-runs find existing rows.
const (
	AdminEmail   = "admin@demo.test"
	MemberEmail  = "ada@demo.test"
	AdminID      = "seed-admin"
	MemberID     = "seed-member"
	CategoryID   = "seed-category-tech"
	ProductID    = "seed-product-zenbook"
	PostID       = "seed-post-welcome"
	DemoPassword = "demo-pass-12"
)

// Run seeds the database: one admin, one member, one category, one product,
// and one post. Seeding is idempotent: existing rows are left untouched.
func Run(ctx context.Context, database db.DB) error {
	passwordargon2.Register()

	if err := ensureUser(ctx, database, AdminID, AdminEmail, "Ada Admin", "admin"); err != nil {
		return err
	}
	if err := ensureUser(ctx, database, MemberID, MemberEmail, "Ada Member", "member"); err != nil {
		return err
	}
	if err := ensureCategory(ctx, database); err != nil {
		return err
	}
	if err := ensureProduct(ctx, database); err != nil {
		return err
	}
	return ensurePost(ctx, database)
}

func ensureUser(ctx context.Context, database db.DB, id, email, name, role string) error {
	_, ok, err := orm.From(gen.Users).Where(gen.UserCols.Email.Eq(email)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup user: %w", err)
	}
	if ok {
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

	if err := orm.InsertInto(gen.Categories).Values(
		orm.Set(gen.CategoryCols.ID, CategoryID),
		orm.Set(gen.CategoryCols.Name, "tech"),
		orm.Set(gen.CategoryCols.Description, "Technology products"),
		orm.Set(gen.CategoryCols.CreatedAt, time.Now().UTC()),
	).Exec(ctx, database); err != nil {
		return fmt.Errorf("[seed] insert category: %w", err)
	}
	return nil
}

func ensureProduct(ctx context.Context, database db.DB) error {
	_, ok, err := orm.From(gen.Products).Where(gen.ProductCols.ID.Eq(ProductID)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup product: %w", err)
	}
	if ok {
		return nil
	}

	if err := orm.InsertInto(gen.Products).Values(
		orm.Set(gen.ProductCols.ID, ProductID),
		orm.Set(gen.ProductCols.CategoryID, CategoryID),
		orm.Set(gen.ProductCols.Name, "ZenBook Pro"),
		orm.Set(gen.ProductCols.Description, "Laptop for builders"),
		orm.Set(gen.ProductCols.PriceCents, int64(129900)),
		orm.Set(gen.ProductCols.Stock, int64(42)),
		orm.Set(gen.ProductCols.CreatedAt, time.Now().UTC()),
	).Exec(ctx, database); err != nil {
		return fmt.Errorf("[seed] insert product: %w", err)
	}
	return nil
}

func ensurePost(ctx context.Context, database db.DB) error {
	_, ok, err := orm.From(gen.Posts).Where(gen.PostCols.ID.Eq(PostID)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup post: %w", err)
	}
	if ok {
		return nil
	}

	if err := orm.InsertInto(gen.Posts).Values(
		orm.Set(gen.PostCols.ID, PostID),
		orm.Set(gen.PostCols.UserID, AdminID),
		orm.Set(gen.PostCols.Title, "Welcome to the Zever demo"),
		orm.Set(gen.PostCols.Body, "Every battery, one schema, zero external services."),
		orm.Set(gen.PostCols.Published, true),
		orm.Set(gen.PostCols.CreatedAt, time.Now().UTC()),
	).Exec(ctx, database); err != nil {
		return fmt.Errorf("[seed] insert post: %w", err)
	}
	return nil
}
