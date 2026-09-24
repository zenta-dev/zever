package main

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
	gen "github.com/zenta-dev/zever/examples/recursive_cte/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

func newTestDB(ctx context.Context, t *testing.T) db.DB {
	t.Helper()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })

	if err := seed(ctx, conn); err != nil {
		t.Fatalf("seed: %v", err)
	}

	return conn
}

func fetchTree(ctx context.Context, t *testing.T, conn db.DB) []*gen.Employee {
	t.Helper()

	name, err := orm.NewCTEName("org_tree")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	got, err := orm.WithRecursive(
		name,
		orm.From(gen.Employees).Where(gen.EmployeeCols.ManagerID.IsNull()),
		orm.Predicate[gen.Employee]{},
		orgTree(name),
	).All(ctx, conn)
	if err != nil {
		t.Fatalf("WithRecursive All: %v", err)
	}

	return got
}

func TestTreeOrder(t *testing.T) {
	ctx := t.Context()
	conn := newTestDB(ctx, t)

	got := fetchTree(ctx, t, conn)
	if len(got) != 7 {
		t.Fatalf("tree size = %d, want 7", len(got))
	}

	roots, children := indexByManager(got)

	if len(roots) != 1 || roots[0].Name != "Ada CEO" {
		t.Fatalf("roots = %v, want exactly [Ada CEO]", names(roots))
	}

	// Depth-first traversal following the sorted children map is the order
	// the demo prints.
	var order []string
	var walk func(e *gen.Employee)
	walk = func(e *gen.Employee) {
		order = append(order, e.Name)
		for _, c := range children[e.ID] {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}

	want := []string{"Ada CEO", "Grace VP", "Margaret Lead", "Barbara Eng", "Radia Eng", "Katherine VP", "Alan Rep"}
	if len(order) != len(want) {
		t.Fatalf("tree order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("tree order = %v, want %v", order, want)
		}
	}
}

func TestHeadcount(t *testing.T) {
	ctx := t.Context()
	conn := newTestDB(ctx, t)

	name, err := orm.NewCTEName("org_tree")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	n, err := orm.WithRecursive(
		name,
		orm.From(gen.Employees).Where(gen.EmployeeCols.ManagerID.IsNull()),
		orm.Predicate[gen.Employee]{},
		orgTree(name),
	).Count(ctx, conn)
	if err != nil {
		t.Fatalf("WithRecursive Count: %v", err)
	}
	if n != 7 {
		t.Fatalf("headcount = %d, want 7", n)
	}
}

func TestSubtree(t *testing.T) {
	ctx := t.Context()
	conn := newTestDB(ctx, t)

	name, err := orm.NewCTEName("vp_subtree")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	got, err := orm.WithRecursive(
		name,
		orm.From(gen.Employees).Where(gen.EmployeeCols.ID.Eq("u2")),
		orm.Predicate[gen.Employee]{},
		orgTree(name),
	).OrderBy(gen.EmployeeCols.Name.Asc()).All(ctx, conn)
	if err != nil {
		t.Fatalf("WithRecursive subtree: %v", err)
	}

	want := []string{"Barbara Eng", "Grace VP", "Margaret Lead", "Radia Eng"}
	if len(got) != len(want) {
		t.Fatalf("subtree = %v, want %v", names(got), want)
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Fatalf("subtree = %v, want %v", names(got), want)
		}
	}
}

func names(es []*gen.Employee) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	return out
}
