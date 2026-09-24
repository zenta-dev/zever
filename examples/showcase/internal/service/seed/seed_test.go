package seed_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	gen "github.com/zenta-dev/zever/examples/showcase/generated/zenorm/orm/gen/shop"
	"github.com/zenta-dev/zever/examples/showcase/internal/service/seed"
	"github.com/zenta-dev/zever/orm"

	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/log/slog"
)

// migrate creates every table from the committed DDL fixture.
func migrate(t *testing.T, database interface {
	Exec(context.Context, string, ...any) (int64, error)
}, dir string) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	ctx := t.Context()
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := database.Exec(ctx, stmt); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
}

func TestRunSeedsAllEntities(t *testing.T) {
	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "seed.db")
	c := container.New(cfg)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	ctx := t.Context()
	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}

	migrate(t, database, filepath.Join("..", "..", "api", "testdata"))

	if runErr := seed.Run(ctx, database); runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	// Idempotent: second run is a no-op.
	if runErr := seed.Run(ctx, database); runErr != nil {
		t.Fatalf("Run again: %v", runErr)
	}

	admin, ok, err := orm.From(gen.Users).Where(gen.UserCols.Email.Eq(seed.AdminEmail)).First(ctx, database)
	if err != nil || !ok {
		t.Fatalf("admin lookup: %v ok=%v", err, ok)
	}
	if admin.Role != gen.RoleAdmin {
		t.Fatalf("admin role = %q, want admin", admin.Role)
	}

	shopper, ok, err := orm.From(gen.Users).Where(gen.UserCols.Email.Eq(seed.ShopperEmail)).First(ctx, database)
	if err != nil || !ok {
		t.Fatalf("shopper lookup: %v ok=%v", err, ok)
	}
	if shopper.Role != gen.RoleMember {
		t.Fatalf("shopper role = %q, want member", shopper.Role)
	}

	products, err := orm.From(gen.Products).All(ctx, database)
	if err != nil {
		t.Fatalf("products: %v", err)
	}
	if len(products) != 2 {
		t.Fatalf("products = %d, want 2", len(products))
	}

	order, ok, err := orm.From(gen.Orders).Where(gen.OrderCols.ID.Eq(seed.OrderID)).First(ctx, database)
	if err != nil || !ok {
		t.Fatalf("order lookup: %v ok=%v", err, ok)
	}
	if order.Status != gen.OrderStatusPending {
		t.Fatalf("order status = %q, want pending", order.Status)
	}
	if note, has := order.Note.Get(); !has || note != "seeded order" {
		t.Fatalf("order note = %q/%v, want seeded order", note, has)
	}

	items, err := orm.From(gen.OrderItems).Where(gen.OrderItemCols.OrderID.Eq(seed.OrderID)).All(ctx, database)
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	if len(items) != 1 || items[0].Quantity != 3 {
		t.Fatalf("items = %+v, want one x3 line", items)
	}

	rows, err := database.Query(ctx, `SELECT COUNT(*) FROM product_tags`)
	if err != nil {
		t.Fatalf("join count: %v", err)
	}
	var links int
	if rows.Next() {
		if err := rows.Scan(&links); err != nil {
			_ = rows.Close()
			t.Fatalf("scan: %v", err)
		}
	}
	_ = rows.Close()
	if links != 2 {
		t.Fatalf("product_tags links = %d, want 2", links)
	}
}
