package firebase

import (
	"path/filepath"
	"testing"
)

// TestValidateServiceAccountPath_uppercaseExtension verifies the extension
// check is case-insensitive.
func TestValidateServiceAccountPath_uppercaseExtension(t *testing.T) {
	t.Parallel()

	p := filepath.Join(t.TempDir(), "sa.JSON")
	if err := ValidateServiceAccountPath(p); err != nil {
		t.Errorf("ValidateServiceAccountPath(%q) = %v, want nil", p, err)
	}
}
