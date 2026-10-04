package dbconn

import (
	"strings"
	"testing"
)

// TestValidateTableName_boundaries covers unicode and punctuation rejects plus
// a very long accepted name.
func TestValidateTableName_boundaries(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"täble", "表", "a.b", "a$b", "a b", "drop;table"} {
		if err := ValidateTableName(name); err == nil {
			t.Errorf("ValidateTableName(%q) = nil, want error", name)
		}
	}

	long := strings.Repeat("a", 4096)
	if err := ValidateTableName(long); err != nil {
		t.Errorf("ValidateTableName(long) = %v, want nil", err)
	}
}
