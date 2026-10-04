package stub

import "testing"

// TestEdgeCreateCustomer_emptyFields covers the empty-name/email boundary:
// the stub assigns an ID and echoes the empty fields rather than rejecting.
func TestEdgeCreateCustomer_emptyFields(t *testing.T) {
	t.Parallel()

	b := New()

	cus, err := b.CreateCustomer(t.Context(), "", "", "")
	if err != nil {
		t.Fatalf("CreateCustomer(empty) = %v, want nil", err)
	}

	if cus.ID == "" {
		t.Fatal("CreateCustomer(empty) returned empty ID")
	}

	if cus.Name != "" || cus.Email != "" {
		t.Fatalf("CreateCustomer(empty) = %+v, want empty name/email", cus)
	}
}
