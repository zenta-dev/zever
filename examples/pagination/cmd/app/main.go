// Command app is the pagination example: a small product catalog
// (schema/pagination.zen, codegen'd by the zenorm backend into
// generated/zenorm/orm/gen/app) queried two ways -- classic OFFSET
// pagination (OrderBy/Limit/Offset + Count, via orm.OffsetPage) and keyset
// (cursor) pagination driven by orm/cursor.go.
//
// The keyset position is the multi-column tuple (price_cents, id): price is
// the sort key, id the deterministic tie-breaker. orm.AfterTuple builds the
// "strictly after" predicate for the tuple, and an opaque page token comes
// from orm.NewCursorKey(...).Encode, verified on every hop with
// orm.DecodeCursor (empty token means "first page"; a token replayed
// against the wrong column fails). Everything stays on orm's immutable
// Query chain.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
	gen "github.com/zenta-dev/zever/examples/pagination/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

const pageSize = 5

func main() {
	ctx := context.Background()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		die(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if err := seed(ctx, conn); err != nil {
		die(err)
	}

	if _, _, err := offsetPagination(ctx, conn); err != nil {
		die(err)
	}

	if _, err := keysetPagination(ctx, conn); err != nil {
		die(err)
	}
}

// seed creates the products table and inserts 25 products. Prices repeat on
// purpose: the keyset tie-break (id after equal price) is only meaningful
// when the sort key has duplicates.
func seed(ctx context.Context, conn db.DB) error {
	if _, err := conn.Exec(ctx, `CREATE TABLE products (id text, name text, price_cents integer, created_at timestamp)`); err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	for i := 1; i <= 25; i++ {
		insert := orm.InsertInto(gen.Products).Values(
			orm.Set(gen.ProductCols.ID, fmt.Sprintf("p%02d", i)),
			orm.Set(gen.ProductCols.Name, fmt.Sprintf("gadget-%02d", i)),
			orm.Set(gen.ProductCols.PriceCents, int64((i%7)*250+499)), // duplicates every 7th price
			orm.Set(gen.ProductCols.CreatedAt, now.Add(time.Duration(i)*time.Hour)),
		)

		if err := insert.Exec(ctx, conn); err != nil {
			return fmt.Errorf("insert p%02d: %w", i, err)
		}
	}

	return nil
}

// priceAscIDAsc is the ORDER BY both pagination styles share: price is the
// sort key, id the deterministic tie-breaker. The keyset predicate below
// must mirror it exactly or pages can skip/dup rows.
func priceAscIDAsc() []orm.OrderTerm[gen.Product] {
	return []orm.OrderTerm[gen.Product]{
		gen.ProductCols.PriceCents.Asc(),
		gen.ProductCols.ID.Asc(),
	}
}

// offsetPagination fetches page 2 of 5 rows plus the total count, the
// classic "page number" UI shape, through orm.OffsetPage (LIMIT/OFFSET) and
// Query.Count.
func offsetPagination(ctx context.Context, conn db.DB) (int64, []*gen.Product, error) {
	total, err := orm.From(gen.Products).Count(ctx, conn)
	if err != nil {
		return 0, nil, fmt.Errorf("count: %w", err)
	}

	q, err := orm.OffsetPage(orm.From(gen.Products).OrderBy(priceAscIDAsc()...), 2, pageSize)
	if err != nil {
		return 0, nil, fmt.Errorf("offset page: %w", err)
	}

	page, err := q.All(ctx, conn)
	if err != nil {
		return 0, nil, fmt.Errorf("offset page: %w", err)
	}

	fmt.Printf("offset page 2 of %d total products (LIMIT %d OFFSET %d):\n", total, pageSize, pageSize)

	for _, p := range page {
		fmt.Printf("  %s %-10s %4d.%02d\n", p.ID, p.Name, p.PriceCents/100, p.PriceCents%100)
	}

	return total, page, nil
}

// afterTuple returns the keyset predicate "strictly after (price, id) in
// (price_cents ASC, id ASC) order": the row-value comparison
// (price_cents, id) > (?, ?), built by orm.AfterTuple from the same typed
// columns every other filter uses.
func afterTuple(price int64, id string) (orm.Predicate[gen.Product], error) {
	after, err := orm.AfterTuple(
		[]orm.AnyColumn[gen.Product]{gen.ProductCols.PriceCents.Col(), gen.ProductCols.ID.Col()},
		[]any{price, id},
		false,
	)
	if err != nil {
		return orm.Predicate[gen.Product]{}, fmt.Errorf("keyset predicate: %w", err)
	}

	return after, nil
}

// keysetPagination walks every page via an opaque cursor. Unlike OFFSET,
// the WHERE clause itself selects the window, so it stays correct when
// rows are inserted or deleted between page loads. The last row of each
// page becomes the next position; a short final page ends the walk.
//
// The position travels two ways: the (price, id) tuple drives the next
// query through afterTuple, and the last row's id travels to the "client"
// as an opaque orm.CursorKey token, decoded back (and round-trip checked)
// before the next fetch.
func keysetPagination(ctx context.Context, conn db.DB) ([][]*gen.Product, error) {
	fmt.Printf("keyset walk (page size %d, ascending price then id):\n", pageSize)

	var pages [][]*gen.Product

	token := ""
	var price int64
	var id string

	for pageNo := 1; ; pageNo++ {
		q := orm.From(gen.Products).OrderBy(priceAscIDAsc()...).Limit(pageSize)

		if pageNo > 1 {
			after, err := afterTuple(price, id)
			if err != nil {
				return nil, err
			}

			q = q.Where(after)
		}

		page, err := q.All(ctx, conn)
		if err != nil {
			return nil, fmt.Errorf("keyset page %d: %w", pageNo, err)
		}

		if len(page) == 0 {
			break
		}

		fmt.Printf("  page %d (cursor %s):", pageNo, token)

		for _, p := range page {
			fmt.Printf(" %s:%d", p.ID, p.PriceCents)
		}

		fmt.Println()

		pages = append(pages, page)
		last := page[len(page)-1]

		next, err := orm.NewCursorKey(gen.ProductCols.ID, last.ID).Encode()
		if err != nil {
			return nil, fmt.Errorf("encode cursor: %w", err)
		}

		decoded, ok, err := orm.DecodeCursor(next, gen.ProductCols.ID)
		if err != nil {
			return nil, fmt.Errorf("decode cursor: %w", err)
		}

		if !ok || decoded.Value() != last.ID {
			return nil, fmt.Errorf("cursor round-trip mismatch for %s", last.ID)
		}

		token, price, id = next, last.PriceCents, last.ID

		if len(page) < pageSize {
			break
		}
	}

	return pages, nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
