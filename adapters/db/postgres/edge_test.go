package postgres

import (
	"strconv"
	"strings"
	"testing"
)

// TestEdgeReplacePlaceholders_manyPlaceholders covers the high-count boundary:
// numbering must stay sequential and correct past single digits.
func TestEdgeReplacePlaceholders_manyPlaceholders(t *testing.T) {
	t.Parallel()

	const n = 100

	var q strings.Builder

	q.WriteString("SELECT * FROM t WHERE")

	want := strings.Builder{}

	want.WriteString("SELECT * FROM t WHERE")

	for i := 1; i <= n; i++ {
		if i > 1 {
			q.WriteString(" AND")
			want.WriteString(" AND")
		}

		q.WriteString(" c = ?")

		want.WriteString(" c = $")
		want.WriteString(strconv.Itoa(i))
	}

	if got := replacePlaceholders(q.String()); got != want.String() {
		t.Fatalf("replacePlaceholders(%d placeholders) mismatch", n)
	}
}
