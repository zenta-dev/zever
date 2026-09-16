package render

import (
	"reflect"
	"strings"
	"testing"
)

// TestMergeRendersUpdateAndInsert is the canonical MERGE shape: an equi-join
// ON, a WHEN MATCHED UPDATE mixing a source-column reference and a literal
// bind, and a WHEN NOT MATCHED INSERT projecting source columns.
func TestMergeRendersUpdateAndInsert(t *testing.T) {
	t.Parallel()
	on := []JoinSpec{{ParentCol: "id", ChildTable: "widget_staging", ChildCol: "widget_id"}}

	whens := []MergeWhen{
		{
			Matched: true,
			Action:  MergeUpdate,
			Sets: []MergeAssignment{
				{Column: "name", Source: "name"},
				{Column: "quantity", Value: int64(5)},
			},
		},
		{
			Action: MergeInsert,
			Sets: []MergeAssignment{
				{Column: "id", Source: "widget_id"},
				{Column: "name", Source: "name"},
			},
		},
	}

	q, args, err := Merge(fakePostgres{}, "widgets", "widget_staging", on, whens)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	want := `MERGE INTO "widgets" USING "widget_staging" ON "widgets"."id" = "widget_staging"."widget_id" ` +
		`WHEN MATCHED THEN UPDATE SET "name" = "widget_staging"."name", "quantity" = $1 ` +
		`WHEN NOT MATCHED THEN INSERT ("id", "name") VALUES ("widget_staging"."widget_id", "widget_staging"."name")`

	if q != want {
		t.Fatalf("query =\n%q\nwant\n%q", q, want)
	}

	if !reflect.DeepEqual(args, []any{int64(5)}) {
		t.Fatalf("args = %#v, want [5]", args)
	}
}

// TestMergeRendersMatchedDelete proves a WHEN MATCHED THEN DELETE clause.
func TestMergeRendersMatchedDelete(t *testing.T) {
	t.Parallel()
	on := []JoinSpec{{ParentCol: "id", ChildTable: "widget_staging", ChildCol: "widget_id"}}
	whens := []MergeWhen{{Matched: true, Action: MergeDelete}}

	q, args, err := Merge(fakePostgres{}, "widgets", "widget_staging", on, whens)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	want := `MERGE INTO "widgets" USING "widget_staging" ON "widgets"."id" = "widget_staging"."widget_id" WHEN MATCHED THEN DELETE`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if args != nil {
		t.Fatalf("args = %#v, want nil", args)
	}
}

// TestMergeRendersInsertDefaultValues proves a WHEN NOT MATCHED INSERT with
// no target assignments renders DEFAULT VALUES.
func TestMergeRendersInsertDefaultValues(t *testing.T) {
	t.Parallel()
	on := []JoinSpec{{ParentCol: "id", ChildTable: "widget_staging", ChildCol: "widget_id"}}
	whens := []MergeWhen{{Action: MergeInsert}}

	q, _, err := Merge(fakePostgres{}, "widgets", "widget_staging", on, whens)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	want := `MERGE INTO "widgets" USING "widget_staging" ON "widgets"."id" = "widget_staging"."widget_id" WHEN NOT MATCHED THEN INSERT DEFAULT VALUES`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

// TestMergeRendersMultipleOnConditions proves multiple ON pairs AND together.
func TestMergeRendersMultipleOnConditions(t *testing.T) {
	t.Parallel()
	on := []JoinSpec{
		{ParentCol: "tenant_id", ChildTable: "widget_staging", ChildCol: "tenant_id"},
		{ParentCol: "id", ChildTable: "widget_staging", ChildCol: "widget_id"},
	}
	whens := []MergeWhen{{Matched: true, Action: MergeDelete}}

	q, _, err := Merge(fakePostgres{}, "widgets", "widget_staging", on, whens)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	if !strings.Contains(q, `ON "widgets"."tenant_id" = "widget_staging"."tenant_id" AND "widgets"."id" = "widget_staging"."widget_id"`) {
		t.Fatalf("query = %q, want both ON conditions ANDed", q)
	}
}

// TestMergeFailsClosed proves the renderer rejects a MERGE missing its ON
// condition or any WHEN clause, and an UPDATE clause with no assignments --
// never emitting invalid SQL.
func TestMergeFailsClosed(t *testing.T) {
	t.Parallel()
	on := []JoinSpec{{ParentCol: "id", ChildTable: "widget_staging", ChildCol: "widget_id"}}

	cases := []struct {
		name  string
		on    []JoinSpec
		whens []MergeWhen
	}{
		{"no-on", nil, []MergeWhen{{Matched: true, Action: MergeDelete}}},
		{"no-whens", on, nil},
		{"update-no-sets", on, []MergeWhen{{Matched: true, Action: MergeUpdate}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := Merge(fakePostgres{}, "widgets", "widget_staging", tc.on, tc.whens); err == nil {
				t.Fatalf("Merge succeeded, want an error")
			}
		})
	}
}

func TestMergeActionValidation(t *testing.T) {
	t.Parallel()

	on := []JoinSpec{{ParentCol: "id", ChildTable: "widget_staging", ChildCol: "widget_id"}}

	t.Run("empty target and source rejected", func(t *testing.T) {
		t.Parallel()

		if _, _, err := Merge(fakePostgres{}, "", "widget_staging", on,
			[]MergeWhen{{Matched: true, Action: MergeDelete}}); err == nil {
			t.Fatal("err = nil, want a missing-table error")
		}

		if _, _, err := Merge(fakePostgres{}, "widgets", "", on,
			[]MergeWhen{{Matched: true, Action: MergeDelete}}); err == nil {
			t.Fatal("err = nil, want a missing-table error")
		}
	})

	t.Run("update in not-matched arm rejected", func(t *testing.T) {
		t.Parallel()

		whens := []MergeWhen{{Action: MergeUpdate, Sets: []MergeAssignment{{Column: "name", Value: "x"}}}}

		if _, _, err := Merge(fakePostgres{}, "widgets", "widget_staging", on, whens); err == nil {
			t.Fatal("err = nil, want an UPDATE-in-NOT-MATCHED error")
		}
	})

	t.Run("delete in not-matched arm rejected", func(t *testing.T) {
		t.Parallel()

		whens := []MergeWhen{{Action: MergeDelete}}

		if _, _, err := Merge(fakePostgres{}, "widgets", "widget_staging", on, whens); err == nil {
			t.Fatal("err = nil, want a DELETE-in-NOT-MATCHED error")
		}
	})

	t.Run("delete with assignments rejected", func(t *testing.T) {
		t.Parallel()

		whens := []MergeWhen{{Matched: true, Action: MergeDelete, Sets: []MergeAssignment{{Column: "name", Value: "x"}}}}

		if _, _, err := Merge(fakePostgres{}, "widgets", "widget_staging", on, whens); err == nil {
			t.Fatal("err = nil, want a DELETE-with-assignments error")
		}
	})

	t.Run("insert in matched arm rejected", func(t *testing.T) {
		t.Parallel()

		whens := []MergeWhen{{Matched: true, Action: MergeInsert}}

		if _, _, err := Merge(fakePostgres{}, "widgets", "widget_staging", on, whens); err == nil {
			t.Fatal("err = nil, want an INSERT-in-MATCHED error")
		}
	})

	t.Run("unknown action rejected", func(t *testing.T) {
		t.Parallel()

		whens := []MergeWhen{{Matched: true, Action: MergeAction(99)}}

		if _, _, err := Merge(fakePostgres{}, "widgets", "widget_staging", on, whens); err == nil {
			t.Fatal("err = nil, want an unknown-action error")
		}
	})
}
