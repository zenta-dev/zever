package main

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	gen "github.com/zenta-dev/zever/examples/pagination/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

func openTestDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	if err := seed(ctx, conn); err != nil {
		t.Fatalf("seed: %v", err)
	}

	return ctx, conn
}

func pageIDs[T any](rows []*T, id func(*T) string) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = id(r)
	}

	return ids
}

func assertIDs(t *testing.T, what string, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

// TestOffsetPageTwoContents checks the total count and the same page-2 IDs
// the demo prints.
func TestOffsetPageTwoContents(t *testing.T) {
	ctx, conn := openTestDB(t)

	total, page, err := offsetPagination(ctx, conn)
	if err != nil {
		t.Fatalf("offset pagination: %v", err)
	}

	if total != 25 {
		t.Fatalf("total = %d, want 25", total)
	}

	assertIDs(t, "offset page 2", pageIDs(page, func(p *gen.Product) string { return p.ID }),
		[]string{"p15", "p22", "p02", "p09", "p16"})
}

// TestKeysetWalkVisitsAllOnce walks every keyset page and proves all 25
// products are visited exactly once, in (price, id) order.
func TestKeysetWalkVisitsAllOnce(t *testing.T) {
	ctx, conn := openTestDB(t)

	pages, err := keysetPagination(ctx, conn)
	if err != nil {
		t.Fatalf("keyset pagination: %v", err)
	}

	if len(pages) != 5 {
		t.Fatalf("pages = %d, want 5", len(pages))
	}

	seen := make(map[string]int, 25)
	var order []string

	for i, page := range pages {
		if len(page) != pageSize {
			t.Fatalf("page %d has %d rows, want %d", i+1, len(page), pageSize)
		}

		for _, p := range page {
			seen[p.ID]++
			order = append(order, p.ID)
		}
	}

	if len(seen) != 25 {
		t.Fatalf("walk visited %d distinct products, want 25", len(seen))
	}

	for id, n := range seen {
		if n != 1 {
			t.Fatalf("product %s visited %d times, want exactly once", id, n)
		}
	}

	// Spot-check the ordering the demo prints: cheapest first, id
	// tie-break, most expensive last.
	assertIDs(t, "keyset page 1", order[:5], []string{"p07", "p14", "p21", "p01", "p08"})
	assertIDs(t, "keyset page 5", order[20:], []string{"p12", "p19", "p06", "p13", "p20"})
}

// TestKeysetPageMatchesOffsetPage proves both styles return the same page
// contents: the second keyset page equals offset page 2.
func TestKeysetPageMatchesOffsetPage(t *testing.T) {
	ctx, conn := openTestDB(t)

	_, offset, err := offsetPagination(ctx, conn)
	if err != nil {
		t.Fatalf("offset pagination: %v", err)
	}

	pages, err := keysetPagination(ctx, conn)
	if err != nil {
		t.Fatalf("keyset pagination: %v", err)
	}

	assertIDs(t, "keyset page 2",
		pageIDs(pages[1], func(p *gen.Product) string { return p.ID }),
		pageIDs(offset, func(p *gen.Product) string { return p.ID }))
}

// TestCursorTokenRoundTrip checks the opaque page token: it decodes back to
// the same column and value, an empty token means "no cursor yet", and a
// token replayed against the wrong column fails.
func TestCursorTokenRoundTrip(t *testing.T) {
	key := orm.NewCursorKey(gen.ProductCols.ID, "p08")

	token, err := key.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	if token == "" {
		t.Fatal("encoded token is empty")
	}

	decoded, ok, err := orm.DecodeCursor(token, gen.ProductCols.ID)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !ok {
		t.Fatal("decode ok = false, want true")
	}

	if decoded.Value() != "p08" {
		t.Fatalf("decoded value = %q, want %q", decoded.Value(), "p08")
	}

	if _, emptyOK, emptyErr := orm.DecodeCursor("", gen.ProductCols.ID); emptyErr != nil || emptyOK {
		t.Fatalf("empty token decode = (%v, %v), want (false, nil)", emptyOK, emptyErr)
	}

	if _, _, nameErr := orm.DecodeCursor(token, gen.ProductCols.Name); nameErr == nil {
		t.Fatal("decoding an id token against the name column succeeded, want error")
	}

	desc := orm.NewCursorKey(gen.ProductCols.PriceCents, int64(749)).Desc()
	descToken, err := desc.Encode()
	if err != nil {
		t.Fatalf("encode desc: %v", err)
	}

	decodedDesc, ok, err := orm.DecodeCursor(descToken, gen.ProductCols.PriceCents)
	if err != nil || !ok {
		t.Fatalf("decode desc = (%v, %v), want (true, nil)", ok, err)
	}

	if !decodedDesc.Descending() || decodedDesc.Value() != 749 {
		t.Fatalf("decoded desc = (descending=%v, value=%d), want (true, 749)",
			decodedDesc.Descending(), decodedDesc.Value())
	}
}
