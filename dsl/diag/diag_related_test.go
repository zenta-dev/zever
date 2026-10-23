package diag

import (
	"errors"
	"testing"
)

// TestWrapRelatedSetsRelated proves WrapRelated carries the given related
// locations alongside the wrapped cause.
func TestWrapRelatedSetsRelated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		related []RelatedInfo
	}{
		{
			name: "one related location",
			related: []RelatedInfo{
				{Pos: Position{File: "a.zen", Line: 1, Col: 8}, Msg: "first declared here"},
			},
		},
		{
			name:    "nil related stays nil",
			related: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cause := errors.New("boom")

			got := WrapRelated("resolve", Position{File: "b.zen", Line: 4, Col: 8}, cause, tc.related,
				"duplicate top-level declaration %q", "Dup")
			if got == nil {
				t.Fatal("WrapRelated() returned nil")
			}

			if len(got.Related) != len(tc.related) {
				t.Fatalf("Related len = %d, want %d", len(got.Related), len(tc.related))
			}

			for i, want := range tc.related {
				if got.Related[i] != want {
					t.Fatalf("Related[%d] = %+v, want %+v", i, got.Related[i], want)
				}
			}

			if !errors.Is(got.Wrapped, cause) {
				t.Fatalf("Wrapped = %v, want %v", got.Wrapped, cause)
			}

			if got.Msg != `duplicate top-level declaration "Dup"` {
				t.Fatalf("Msg = %q, want the formatted message", got.Msg)
			}
		})
	}
}

// TestNewHasNoRelated proves diagnostics built without WrapRelated default
// to no related locations.
func TestNewHasNoRelated(t *testing.T) {
	t.Parallel()

	if got := New("parse", Position{File: "a.zen", Line: 1, Col: 1}, "boom"); len(got.Related) != 0 {
		t.Fatalf("New().Related = %+v, want empty", got.Related)
	}

	if got := Wrap("resolve", Position{File: "a.zen", Line: 1, Col: 1}, errors.New("boom"), "boom"); len(
		got.Related) != 0 {
		t.Fatalf("Wrap().Related = %+v, want empty", got.Related)
	}
}

// TestRelatedDoesNotAffectErrorOrSorted proves related locations are
// out-of-band metadata: Error renders only the primary position and Sorted
// orders only by it.
func TestRelatedDoesNotAffectErrorOrSorted(t *testing.T) {
	t.Parallel()

	related := []RelatedInfo{{Pos: Position{File: "a.zen", Line: 1, Col: 1}, Msg: "first declared here"}}

	with := WrapRelated(
		"resolve", Position{File: "b.zen", Line: 2, Col: 3}, nil, related, "boom")
	without := New("resolve", Position{File: "b.zen", Line: 2, Col: 3}, "boom")

	if with.Error() != without.Error() {
		t.Fatalf("Error() with Related = %q, want %q (without)", with.Error(), without.Error())
	}

	list := List{with}
	sorted := list.Sorted()

	if len(sorted) != 1 || sorted[0] != with {
		t.Fatalf("Sorted() = %+v, want the single diagnostic untouched", sorted)
	}

	other := New("resolve", Position{File: "b.zen", Line: 1, Col: 1}, "earlier")
	mixed := List{with, other}.Sorted()

	if mixed[0] != other || mixed[1] != with {
		t.Fatalf(
			"Sorted() with Related = [%v %v], want primary-Pos order",
			mixed[0].Msg, mixed[1].Msg,
		)
	}
}
