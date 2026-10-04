package main

import "testing"

// TestChunkGroupsNonPositive pins the maxGroups boundary: zero or negative
// values collapse to a single group instead of dividing by zero.
func TestChunkGroupsNonPositive(t *testing.T) {
	t.Parallel()

	mods := []string{"a", "b", "c"}
	for _, maxGroups := range []int{0, -1, -100} {
		got := chunkGroups(mods, maxGroups)
		mustEqual(t, "chunkGroups", got, [][]string{{"a", "b", "c"}})
	}
}

// TestDependentsClosureCycle pins that a dependency cycle terminates and
// includes every node exactly once.
func TestDependentsClosureCycle(t *testing.T) {
	t.Parallel()

	requires := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}
	got := dependentsClosure([]string{"a"}, requires)
	mustEqual(t, "cycle closure", got, []string{"a", "b", "c"})
}

// TestDependentsClosureDuplicates pins that duplicate change entries collapse
// to a single node.
func TestDependentsClosureDuplicates(t *testing.T) {
	t.Parallel()

	got := dependentsClosure([]string{"a", "a", "b", "b"}, map[string][]string{"c": {"a"}})
	mustEqual(t, "dedup", got, []string{"a", "b", "c"})
}
