package shop_test

import (
	"testing"

	genshop "github.com/zenta-dev/zever/examples/showcase/generated/gogen/shop"
	"github.com/zenta-dev/zever/shared/apperror"
)

// TestGetProductNilRequest pins the nil-request boundary.
func TestGetProductNilRequest(t *testing.T) {
	s := newTestSetup(t)

	if _, err := s.svc.GetProduct(t.Context(), nil); codeOf(t, err) != apperror.InvalidArgument {
		t.Fatalf("GetProduct(nil) = %v, want InvalidArgument", err)
	}
}

// TestListProductsInvalidCursor pins the malformed-cursor error path.
func TestListProductsInvalidCursor(t *testing.T) {
	s := newTestSetup(t)

	for _, cursor := range []string{"abc", "-1", "1.5"} {
		if _, err := s.svc.ListProducts(t.Context(), nil, cursor, 10); codeOf(t, err) != apperror.InvalidArgument {
			t.Fatalf("ListProducts(cursor=%q) = %v, want InvalidArgument", cursor, err)
		}
	}
}

// TestPatchProductNegativeStock pins the negative-stock boundary.
func TestPatchProductNegativeStock(t *testing.T) {
	s := newTestSetup(t)

	_, err := s.svc.PatchProduct(t.Context(), &genshop.PatchProductRequest{Id: "missing", Stock: -1})
	if codeOf(t, err) != apperror.InvalidArgument {
		t.Fatalf("PatchProduct(negative stock) = %v, want InvalidArgument", err)
	}
}
