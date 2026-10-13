package orm

import (
	"errors"
	"fmt"
	"testing"
)

// bigIDSet builds an IN list far beyond any dialect's parameter budget:
// 40,000 ids (live sqlite allows 32766, postgres 65535) with the three
// seeded widget ids spread across chunk boundaries so fan-out
// concatenation is proven, not just the absence of an error.
func bigIDSet() []string {
	const n = 40000

	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("absent-%05d", i)
	}

	ids[0] = "w1"
	ids[20000] = "w2"
	ids[n-1] = "w3"

	return ids
}

// TestQueryAllAutoChunksLargeInList proves a value-list IN larger than the
// dialect's parameter budget fans out into multiple statements instead of
// failing with the driver's "too many SQL variables" error.
func TestQueryAllAutoChunksLargeInList(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rows, err := From(widgets).Where(widgetID.In(bigIDSet()...)).All(ctx, conn)
	if err != nil {
		t.Fatalf("All with 40000-id IN: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("All with 40000-id IN returned %d rows, want 3", len(rows))
	}

	got := map[string]bool{}
	for _, r := range rows {
		got[r.ID] = true
	}

	for _, want := range []string{"w1", "w2", "w3"} {
		if !got[want] {
			t.Errorf("missing row %q in %+v", want, got)
		}
	}
}

// TestQueryCountAutoChunksLargeInList proves Count sums across the fan-out.
func TestQueryCountAutoChunksLargeInList(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	n, err := From(widgets).Where(widgetID.In(bigIDSet()...)).Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count with 40000-id IN: %v", err)
	}

	if n != 3 {
		t.Fatalf("Count with 40000-id IN = %d, want 3", n)
	}
}

// TestQueryAllChunkedInListRejectsModifiers proves fan-out fails closed
// when result-shaping modifiers make multi-statement concatenation
// semantics-unsafe: ORDER BY across chunks has no global order, and
// LIMIT/OFFSET apply to the whole result, not per chunk.
func TestQueryAllChunkedInListRejectsModifiers(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	ids := bigIDSet()

	if _, err := From(widgets).Where(widgetID.In(ids...)).OrderBy(widgetID.Asc()).All(ctx, conn); !errors.Is(err, ErrInListTooLarge) {
		t.Fatalf("ordered oversized IN err = %v, want ErrInListTooLarge", err)
	}

	if _, err := From(widgets).Where(widgetID.In(ids...)).Limit(5).All(ctx, conn); !errors.Is(err, ErrInListTooLarge) {
		t.Fatalf("limited oversized IN err = %v, want ErrInListTooLarge", err)
	}

	if _, err := From(widgets).Where(widgetID.In(ids...)).Offset(5).All(ctx, conn); !errors.Is(err, ErrInListTooLarge) {
		t.Fatalf("offset oversized IN err = %v, want ErrInListTooLarge", err)
	}
}

// TestQueryAllChunkedInListRejectsNegation proves an oversized IN under
// NOT (or OR) fails closed: chunking a negated IN per chunk would
// intersect wrongly, so it is rejected instead of mis-executed.
func TestQueryAllChunkedInListRejectsNegation(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	ids := bigIDSet()

	if _, err := From(widgets).Where(Not(widgetID.In(ids...))).All(ctx, conn); !errors.Is(err, ErrInListTooLarge) {
		t.Fatalf("negated oversized IN err = %v, want ErrInListTooLarge", err)
	}
}
