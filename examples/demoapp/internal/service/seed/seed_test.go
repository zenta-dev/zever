package seed_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	gen "github.com/zenta-dev/zever/examples/demoapp/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/examples/demoapp/internal/service/seed"
	"github.com/zenta-dev/zever/orm"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
)

// migrate creates every table from the committed DDL fixture.
func migrate(t *testing.T, database interface {
	Exec(context.Context, string, ...any) (int64, error)
}) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "testdata", "schema.sql"))
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

// TestRunSeedsAndIsIdempotent seeds twice and asserts the fixed demo rows
// exist exactly once.
func TestRunSeedsAndIsIdempotent(t *testing.T) {
	dbsqlite.Register()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "seed.db")
	c := container.New(cfg)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	ctx := t.Context()
	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}

	migrate(t, database)

	if runErr := seed.Run(ctx, database); runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	if runErr := seed.Run(ctx, database); runErr != nil {
		t.Fatalf("Run again: %v", runErr)
	}

	admin, ok, err := orm.From(gen.Users).Where(gen.UserCols.Email.Eq(seed.AdminEmail)).First(ctx, database)
	if err != nil || !ok {
		t.Fatalf("admin lookup: %v ok=%v", err, ok)
	}

	if admin.Role != "admin" {
		t.Fatalf("admin role = %q, want admin", admin.Role)
	}

	member, ok, err := orm.From(gen.Users).Where(gen.UserCols.Email.Eq(seed.MemberEmail)).First(ctx, database)
	if err != nil || !ok {
		t.Fatalf("member lookup: %v ok=%v", err, ok)
	}

	if member.Role != "member" {
		t.Fatalf("member role = %q, want member", member.Role)
	}

	products, err := orm.From(gen.Products).All(ctx, database)
	if err != nil {
		t.Fatalf("products: %v", err)
	}

	if len(products) != 1 || products[0].ID != seed.ProductID {
		t.Fatalf("products = %+v, want one seeded product", products)
	}

	posts, err := orm.From(gen.Posts).All(ctx, database)
	if err != nil {
		t.Fatalf("posts: %v", err)
	}

	if len(posts) != 1 || posts[0].ID != seed.PostID {
		t.Fatalf("posts = %+v, want one seeded post", posts)
	}
}
