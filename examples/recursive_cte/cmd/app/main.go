// Command app is the recursive-CTE example: a self-referential org chart
// (schema/recursive_cte.zen, codegen'd by the zenorm backend into
// generated/zenorm/orm/gen/app) walked in a SINGLE `WITH RECURSIVE` query
// built through orm.WithRecursive — no N+1 preloads, no client-side walking
// of manager links to discover the reachable set.
//
// It seeds a small tree, proves the recursion returns every employee
// reachable from the root (plus the root's own filter), counts the whole
// org through the same CTE, and shows a subtree query — everyone managed,
// directly or transitively, by one VP.
package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
	gen "github.com/zenta-dev/zever/examples/recursive_cte/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

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

	if err := walkOrgChart(ctx, conn); err != nil {
		die(err)
	}

	if err := countOrg(ctx, conn); err != nil {
		die(err)
	}

	if err := walkSubtree(ctx, conn); err != nil {
		die(err)
	}
}

// seed creates the employees table and inserts a seven-person org chart. The
// seeds use orm.Insert/orm.Set so every build artifact — schema, codegen,
// mutations, the recursive CTE — comes from the same typed surface.
func seed(ctx context.Context, conn db.DB) error {
	if _, err := conn.Exec(ctx, `CREATE TABLE employees (id text, name text, title text, manager_id text)`); err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	// (id, name, title, managerID)
	people := []struct {
		id, name, title string
		manager         string // "" means no manager (the CEO)
	}{
		{"u1", "Ada CEO", "Chief Executive Officer", ""},
		{"u2", "Grace VP", "VP Engineering", "u1"},
		{"u3", "Katherine VP", "VP Sales", "u1"},
		{"u4", "Margaret Lead", "Engineering Lead", "u2"},
		{"u5", "Radia Eng", "Engineer", "u4"},
		{"u6", "Barbara Eng", "Engineer", "u4"},
		{"u7", "Alan Rep", "Sales Representative", "u3"},
	}

	for _, p := range people {
		var manager orm.Assignment[gen.Employee]

		if p.manager == "" {
			manager = gen.EmployeeCols.ManagerID.SetNull()
		} else {
			manager = gen.EmployeeCols.ManagerID.SetValue(p.manager)
		}

		insert := orm.InsertInto(gen.Employees).Values(
			orm.Set(gen.EmployeeCols.ID, p.id),
			orm.Set(gen.EmployeeCols.Name, p.name),
			orm.Set(gen.EmployeeCols.Title, p.title),
			manager,
		)

		if err := insert.Exec(ctx, conn); err != nil {
			return fmt.Errorf("insert %s: %w", p.name, err)
		}
	}

	return nil
}

// orgTree is the CTE descriptor shared by every recursive query: it joins
// employees.manager_id back to the CTE's own id column. The key names come
// from the codegen'd typed columns (Col().Name()), never bare strings, and
// the CTE reference is an orm.CTETable pointing at the very name the
// WithRecursive call creates.
func orgTree(name orm.CTEName) orm.Relation[gen.Employee, gen.Employee] {
	return orm.NewRelation[gen.Employee, gen.Employee](
		gen.EmployeeCols.ManagerID.Col().Name(),
		gen.EmployeeCols.ID.Col().Name(),
		orm.CTETable[gen.Employee](name),
	)
}

// walkOrgChart runs the recursive CTE over the whole org and prints it as
// an indented tree, built client-side from the flat recursive result.
func walkOrgChart(ctx context.Context, conn db.DB) error {
	name, err := orm.NewCTEName("org_tree")
	if err != nil {
		return err
	}

	everyone, err := orm.WithRecursive(
		name,
		orm.From(gen.Employees).Where(gen.EmployeeCols.ManagerID.IsNull()), // anchor: the root(s)
		orm.Predicate[gen.Employee]{},                                      // recursive branch has no extra filter
		orgTree(name),
	).All(ctx, conn)
	if err != nil {
		return fmt.Errorf("recursive CTE: %w", err)
	}

	roots, children := indexByManager(everyone)

	fmt.Printf("org chart (%d employees via one WITH RECURSIVE query):\n", len(everyone))

	for _, root := range roots {
		printNode(root, children, 0)
	}

	return nil
}

// countOrg proves Count works over the same recursive CTE.
func countOrg(ctx context.Context, conn db.DB) error {
	name, err := orm.NewCTEName("org_tree")
	if err != nil {
		return err
	}

	n, err := orm.WithRecursive(
		name,
		orm.From(gen.Employees).Where(gen.EmployeeCols.ManagerID.IsNull()),
		orm.Predicate[gen.Employee]{},
		orgTree(name),
	).Count(ctx, conn)
	if err != nil {
		return fmt.Errorf("recursive CTE count: %w", err)
	}

	fmt.Printf("total headcount via recursive CTE count: %d\n", n)

	return nil
}

// walkSubtree starts the recursion at one VP (id u2, whose row is the
// anchor) and lists everyone managed — directly or transitively — by her,
// proving the outer query can also filter the recursive result.
func walkSubtree(ctx context.Context, conn db.DB) error {
	name, err := orm.NewCTEName("vp_subtree")
	if err != nil {
		return err
	}

	subtree, err := orm.WithRecursive(
		name,
		orm.From(gen.Employees).Where(gen.EmployeeCols.ID.Eq("u2")), // anchor: Grace VP herself
		orm.Predicate[gen.Employee]{},
		orgTree(name),
	).OrderBy(gen.EmployeeCols.Name.Asc()).All(ctx, conn)
	if err != nil {
		return fmt.Errorf("recursive CTE subtree: %w", err)
	}

	fmt.Printf("Grace VP's org (recursive subtree):")

	for _, e := range subtree {
		fmt.Printf(" %s", e.Name)
	}

	fmt.Println()

	return nil
}

// indexByManager splits a flat employee list into roots (no manager) and a
// manager->reports map, each sorted by name for deterministic output.
func indexByManager(people []*gen.Employee) (roots []*gen.Employee, children map[string][]*gen.Employee) {
	children = make(map[string][]*gen.Employee)

	for _, e := range people {
		if mgr, ok := e.ManagerID.Get(); ok {
			children[mgr] = append(children[mgr], e)

			continue
		}

		roots = append(roots, e)
	}

	byName := func(list []*gen.Employee) {
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	}

	byName(roots)

	for _, v := range children {
		byName(v)
	}

	return roots, children
}

func printNode(e *gen.Employee, children map[string][]*gen.Employee, depth int) {
	fmt.Printf("%s%s (%s)\n", indent(depth), e.Name, e.Title)

	for _, c := range children[e.ID] {
		printNode(c, children, depth+1)
	}
}

func indent(depth int) string {
	out := make([]byte, depth*2)
	for i := range out {
		out[i] = ' '
	}

	return string(out)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
