package shop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	genshop "github.com/zenta-dev/zever/examples/showcase/generated/gogen/shop"
	shopsvc "github.com/zenta-dev/zever/examples/showcase/internal/service/shop"
)

// benchService opens a fresh sqlite file, applies the showcase schema, and
// seeds one category with 20 products.
func benchService(b *testing.B) *shopsvc.ShopServiceImpl {
	b.Helper()

	dbsqlite.Register()

	ctx := b.Context()
	conn, err := db.Open(db.SQLite, db.Options{Path: filepath.Join(b.TempDir(), "bench.db")})
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close(b.Context()) })

	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "testdata", "schema.sql"))
	if err != nil {
		b.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := conn.Exec(ctx, stmt); err != nil {
			b.Fatalf("migrate: %v", err)
		}
	}

	const stamp = "2026-01-02T03:04:05Z"
	if _, err := conn.Exec(ctx,
		`INSERT INTO categories (id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		"cat-1", "Gadgets", "gadgets", stamp); err != nil {
		b.Fatalf("seed category: %v", err)
	}
	for i := 0; i < 20; i++ {
		if _, err := conn.Exec(ctx,
			`INSERT INTO products (id, category_id, sku, headline, description, price_cents, stock, weight, featured, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"p-"+string(rune('a'+i)), "cat-1", "sku-"+string(rune('a'+i)), "Widget", "desc", 2500, 10, 1.0, 0, stamp); err != nil {
			b.Fatalf("seed product: %v", err)
		}
	}

	return shopsvc.NewShopServiceImpl(shopsvc.Deps{DB: conn})
}

// BenchmarkListProducts measures the paginated product listing.
func BenchmarkListProducts(b *testing.B) {
	svc := benchService(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		resp, err := svc.ListProducts(ctx, nil, "", 10)
		if err != nil {
			b.Fatalf("ListProducts: %v", err)
		}
		if len(resp.Items) != 10 {
			b.Fatalf("items = %d, want 10", len(resp.Items))
		}
	}
}

// BenchmarkGetProduct measures the single-row product fetch.
func BenchmarkGetProduct(b *testing.B) {
	svc := benchService(b)
	ctx := b.Context()
	req := &genshop.GetProductRequest{Id: "p-a"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := svc.GetProduct(ctx, req); err != nil {
			b.Fatalf("GetProduct: %v", err)
		}
	}
}
