package authz_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/authz"
)

// TestAuthorizeOwnerFieldWiring proves the opt-in OwnerField wiring: empty
// (default) leaves resource Attributes unset so ownership-scoped rules deny
// as before, while a set OwnerField populates Attributes from the resource
// ID so an OwnedOnly rule can match when owner equals resource ID.
func TestAuthorizeOwnerFieldWiring(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ownerField string
		resourceID string
		wantAttrs  map[string]string
	}{
		{name: "default off leaves attributes empty", ownerField: "", resourceID: "u1", wantAttrs: nil},
		{name: "set wires resource id under owner field", ownerField: "user_id", resourceID: "u1", wantAttrs: map[string]string{"user_id": "u1"}},
		{name: "set wires mismatched id verbatim", ownerField: "user_id", resourceID: "other", wantAttrs: map[string]string{"user_id": "other"}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u1"}}
			p := &fakeChecker{allow: true}
			pol := authz.Policy{
				AuthRequired:    true,
				PermissionCheck: "user.read",
				ResourceType:    "User",
				OwnerField:      tc.ownerField,
			}
			if _, err := authz.Authorize(t.Context(), a, p, pol, "tok", tc.resourceID); err != nil {
				t.Fatalf("Authorize() err = %v, want nil", err)
			}
			if len(tc.wantAttrs) == 0 {
				if len(p.gotResource.Attributes) != 0 {
					t.Fatalf("attributes = %v, want empty (default off)", p.gotResource.Attributes)
				}
				return
			}
			if len(p.gotResource.Attributes) != len(tc.wantAttrs) {
				t.Fatalf("attributes = %v, want %v", p.gotResource.Attributes, tc.wantAttrs)
			}
			for k, want := range tc.wantAttrs {
				if p.gotResource.Attributes[k] != want {
					t.Fatalf("attributes[%q] = %q, want %q", k, p.gotResource.Attributes[k], want)
				}
			}
		})
	}
}
