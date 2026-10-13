package orm

import "fmt"

// planInChunks splits where so every statement it describes stays within
// the dialect's per-statement bind-parameter budget. It returns one Node
// per statement to execute: a single-element slice (the input unchanged)
// when the whole where fits, or one Node per IN chunk when the largest
// chunkable value-list IN must be fanned out.
//
// Only a value-list IN with Op In in positive AND-top-level polarity is
// chunkable: the root node itself or nested under CAnd compounds at any
// depth. Anything else oversized fails closed with ErrInListTooLarge:
//   - a value-list NOT IN (any polarity): per-chunk `NOT IN` results would
//     intersect wrongly;
//   - a value-list IN under COr/CNot: a chunk matches a different row set
//     than the whole, so concatenation is wrong;
//   - a subquery IN: its length is unknown until execution.
//
// Non-IN arguments (binary/between/like) are counted exactly; every other
// kind contributes zero, which underestimates exotic shapes -- erring
// toward the historical single-statement behavior, never toward silently
// dropping rows. A shape whose non-chunkable arguments alone already fill
// the budget also fails closed: even a one-element chunk would overflow.
func planInChunks(where Node, budget int) ([]Node, error) {
	if budget < 1 {
		return nil, fmt.Errorf("orm: planInChunks: budget %d: %w", budget, ErrInListTooLarge)
	}

	if countNodeArgs(where) <= budget {
		return []Node{where}, nil
	}

	if oversizedUnchunkableIn(&where, budget) {
		return nil, fmt.Errorf("orm: planInChunks: %w", ErrInListTooLarge)
	}

	return splitWhere(where, budget)
}

// splitWhere fans out where's largest chunkable value-list IN and recurses
// so any remaining oversized IN is split in turn. Each recursion strictly
// shrinks an IN length, so it always terminates.
func splitWhere(where Node, budget int) ([]Node, error) {
	if countNodeArgs(where) <= budget {
		return []Node{where}, nil
	}

	target := largestChunkableIn(&where, true)
	if target == nil {
		return nil, fmt.Errorf("orm: planInChunks: %w", ErrInListTooLarge)
	}

	vals := target.Value.([]any) //nolint:forcetypeassert // largestChunkableIn only returns value-list INs

	others := countNodeArgs(where) - len(vals)
	chunkSize := budget - others

	if chunkSize < 1 || len(vals) <= chunkSize {
		return nil, fmt.Errorf("orm: planInChunks: %w", ErrInListTooLarge)
	}

	var out []Node

	for start := 0; start < len(vals); start += chunkSize {
		end := start + chunkSize
		if end > len(vals) {
			end = len(vals)
		}

		sub, err := splitWhere(replaceInValues(&where, target, vals[start:end]), budget)
		if err != nil {
			return nil, err
		}

		out = append(out, sub...)
	}

	return out, nil
}

// largestChunkableIn returns the longest value-list IN (Op In only) in
// positive AND-top-level polarity, or nil when there is none. chunkable
// tracks polarity: only the root and CAnd-nested nodes qualify; COr/CNot
// children never do.
func largestChunkableIn(n *Node, chunkable bool) *Node {
	var best *Node

	var walk func(n *Node, chunkable bool)
	walk = func(n *Node, chunkable bool) {
		if n.Kind == NIn && n.Op == In && chunkable {
			if vs, ok := n.Value.([]any); ok {
				if best == nil || len(vs) > len(best.Value.([]any)) { //nolint:forcetypeassert // best only ever holds value-list INs
					best = n
				}
			}
		}

		if n.Kind == NCompound {
			for i := range n.Children {
				walk(&n.Children[i], chunkable && n.Compound == CAnd)
			}
		}
	}

	walk(n, chunkable)

	return best
}

// oversizedUnchunkableIn reports whether where holds a value-list IN
// longer than budget that planInChunks can never split (wrong op or wrong
// polarity), so the caller fails closed instead of executing a shape whose
// total stays over budget after fan-out.
func oversizedUnchunkableIn(where *Node, budget int) bool {
	found := false

	var walk func(n *Node, chunkable bool)
	walk = func(n *Node, chunkable bool) {
		if n.Kind == NCompound {
			for i := range n.Children {
				walk(&n.Children[i], chunkable && n.Compound == CAnd)
			}

			return
		}

		if n.Kind != NIn || found {
			return
		}

		vs, ok := n.Value.([]any)
		if !ok || len(vs) <= budget {
			return
		}

		if !chunkable || n.Op != In {
			found = true
		}
	}

	walk(where, true)

	return found
}

// replaceInValues returns a copy of where with target's value list
// swapped for chunk. The copy is path-local: only nodes on the path to
// target are reallocated, so the caller's tree is never mutated. where
// and target must point into the same tree (splitWhere passes &where for
// both), because identity is by pointer.
func replaceInValues(where *Node, target *Node, chunk []any) Node {
	if where == target {
		out := *where
		out.Value = chunk

		return out
	}

	out := *where

	if where.Kind == NCompound {
		children := make([]Node, len(where.Children))
		copy(children, where.Children)

		for i := range children {
			if contains(&children[i], target) {
				children[i] = replaceInValues(&children[i], target, chunk)
			}
		}

		out.Children = children
	}

	return out
}

// contains reports whether target is n or lives under it.
func contains(n, target *Node) bool {
	if n == target {
		return true
	}

	if n.Kind == NCompound {
		for i := range n.Children {
			if contains(&n.Children[i], target) {
				return true
			}
		}
	}

	return false
}

// countNodeArgs returns how many bound placeholders where renders for the
// countable predicate kinds (binary, value-list IN, between, like,
// compounds of those), mirroring render's arg collection. Every other
// kind contributes zero: an approximation documented on planInChunks.
func countNodeArgs(n Node) int {
	switch n.Kind { //nolint:exhaustive // uncountable kinds contribute zero via default
	case NNone:
		return 0
	case NBinary:
		switch n.Op { //nolint:exhaustive // only the two null ops bind nothing; the rest bind one
		case IsNull, IsNotNull:
			return 0
		default:
			return 1
		}
	case NIn:
		if vs, ok := n.Value.([]any); ok {
			return len(vs)
		}

		return 0
	case NBetween:
		return 2
	case NLike:
		return 1
	case NCompound:
		if n.Compound == CNot {
			if len(n.Children) == 1 {
				return countNodeArgs(n.Children[0])
			}

			return 0
		}

		total := 0

		for _, c := range n.Children {
			total += countNodeArgs(c)
		}

		return total
	default:
		return 0
	}
}
