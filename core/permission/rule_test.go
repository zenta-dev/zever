package permission

import (
	"errors"
	"strings"
	"testing"
)

func TestRule_Validate_table(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 257)
	cases := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{"valid allow", Rule{Role: "admin", Action: "read", Effect: Allow}, false},
		{"valid empty effect defaults allow", Rule{Role: "admin", Action: "read"}, false},
		{"valid deny", Rule{Role: "admin", Action: "read", Effect: Deny}, false},
		{"valid owned", Rule{Role: "user", Action: "read", OwnedOnly: true, OwnedAttr: "owner_id"}, false},
		{"empty role", Rule{Action: "read"}, true},
		{"empty action", Rule{Role: "admin"}, true},
		{"bad effect", Rule{Role: "admin", Action: "read", Effect: "maybe"}, true},
		{"owned without attr", Rule{Role: "user", Action: "read", OwnedOnly: true}, true},
		{"overlong role", Rule{Role: long, Action: "read"}, true},
		{"overlong action", Rule{Role: "admin", Action: long}, true},
	}
	for _, c := range cases {
		err := c.rule.Validate()
		if c.wantErr && err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
		if !c.wantErr && err != nil {
			t.Errorf("%s: unexpected err %v", c.name, err)
		}
		if err != nil && !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("%s: err %v does not match ErrInvalidOptions", c.name, err)
		}
	}
}

func TestRule_effect_default(t *testing.T) {
	t.Parallel()
	if got := (Rule{}).effect(); got != Allow {
		t.Fatalf("empty effect = %q want allow", got)
	}
	if got := (Rule{Effect: Deny}).effect(); got != Deny {
		t.Fatalf("deny effect = %q want deny", got)
	}
}
