package orm

import (
	"errors"
	"fmt"
	"testing"
)

// dupAcrossChunksIDs places w1 twice in an oversized IN list so the
// duplicates land in different chunks.
func dupAcrossChunksIDs() []string {
	ids := bigIDSet()
	ids[len(ids)-2] = "w1"

	return ids
}

// TestQueryAllChunkedInListDuplicateAcrossChunks proves a value repeated
// across chunk boundaries yields its row once, via All, Stream and Count.
func TestQueryAllChunkedInListDuplicateAcrossChunks(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	q := From(widgets).Where(widgetID.In(dupAcrossChunksIDs()...))

	rows, err := q.All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("All returned %d rows, want 3 (w1 duplicated across chunks)", len(rows))
	}

	n := 0

	for _, err := range q.Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		n++
	}

	if n != 3 {
		t.Fatalf("Stream yielded %d rows, want 3", n)
	}

	cnt, err := q.Count(ctx, conn)
	if err != nil || cnt != 3 {
		t.Fatalf("Count = %d, %v; want 3, nil", cnt, err)
	}

	ok, err := q.Exists(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("Exists = %v, %v; want true, nil", ok, err)
	}
}

// TestQueryChunkedInListNullValues proves NULL entries in an oversized IN
// list never match rows (SQL three-valued logic) and do not break dedupe.
func TestQueryChunkedInListNullValues(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	bioAny := NewColumn[widget, any]("widgets", "bio")

	vals := make([]any, 0, 40002)
	for i := range 40000 {
		vals = append(vals, fmt.Sprintf("absent-%05d", i))
	}

	vals[0] = nil
	vals[20000] = nil
	vals[30000] = "first"
	vals[39999] = "first"
	vals = append(vals, nil, "third")

	rows, err := From(widgets).Where(bioAny.In(vals...)).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("All returned %d rows, want 2 (NULL matches nothing)", len(rows))
	}

	n, err := From(widgets).Where(bioAny.In(vals...)).Count(ctx, conn)
	if err != nil || n != 2 {
		t.Fatalf("Count = %d, %v; want 2, nil", n, err)
	}
}

// TestPlanInChunksDedupe proves order-preserving dedupe happens before
// chunking, collapses a list that fits after dedupe into one statement,
// and leaves NOT IN shapes failing closed.
func TestPlanInChunksDedupe(t *testing.T) {
	in := Node{Kind: NIn, Column: "id", Op: In, Value: []any{"a", "b", "a", nil, "c", nil, "b", []byte("x")}}

	plans, err := planInChunks(in, 5)
	if err != nil || len(plans) != 1 {
		t.Fatalf("plans = %d, %v; want 1 statement after dedupe", len(plans), err)
	}

	got, _ := plans[0].Value.([]any)
	if len(got) != 5 || got[0] != "a" || got[1] != "b" || got[2] != nil || got[3] != "c" {
		t.Fatalf("deduped list = %v", got)
	}

	big := Node{Kind: NIn, Column: "id", Op: In, Value: []any{"a", "b", "c", "a", "b", "c", "d"}}

	plans, err = planInChunks(big, 2)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[any]int{}

	for _, p := range plans {
		vs, _ := p.Value.([]any)
		for _, v := range vs {
			seen[v]++
		}
	}

	if len(plans) != 2 || len(seen) != 4 {
		t.Fatalf("plans=%d seen=%v; want 2 chunks of 4 distinct values", len(plans), seen)
	}

	for v, c := range seen {
		if c != 1 {
			t.Errorf("value %v appears in %d chunks", v, c)
		}
	}

	notIn := Node{Kind: NIn, Column: "id", Op: NotIn, Value: []any{"a", "a", "b"}}

	_, err = planInChunks(notIn, 2)
	if !errors.Is(err, ErrInListTooLarge) {
		t.Fatalf("NOT IN oversized: err = %v, want ErrInListTooLarge", err)
	}
}
