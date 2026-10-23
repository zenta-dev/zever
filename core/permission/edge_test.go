package permission

import (
	"errors"
	"strings"
	"testing"
)

func TestOptions_Validate_length_boundary(t *testing.T) {
	t.Parallel()

	at256 := strings.Repeat("x", 256)
	at257 := strings.Repeat("x", 257)

	cases := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{"rule role at max", Options{Rules: []Rule{{Role: at256, Action: "read"}}}, false},
		{"rule role over max", Options{Rules: []Rule{{Role: at257, Action: "read"}}}, true},
		{"rule action at max", Options{Rules: []Rule{{Role: "admin", Action: at256}}}, false},
		{"rule action over max", Options{Rules: []Rule{{Role: "admin", Action: at257}}}, true},
		{"role key at max", Options{Roles: map[string][]string{at256: {"user"}}}, false},
		{"role key over max", Options{Roles: map[string][]string{at257: {"user"}}}, true},
		{"role value at max", Options{Roles: map[string][]string{"admin": {at256}}}, false},
		{"role value over max", Options{Roles: map[string][]string{"admin": {at257}}}, true},
		{"nil roles", Options{Roles: nil}, false},
		{"empty role value list", Options{Roles: map[string][]string{"admin": {}}}, false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.opts.Validate()
			if tc.wantErr && !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Validate() err = %v, want ErrInvalidOptions", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() err = %v, want nil", err)
			}
		})
	}
}

func TestSubject_roundtrip_zero(t *testing.T) {
	t.Parallel()

	ctx := WithSubject(t.Context(), Subject{})
	got, ok := SubjectFrom(ctx)
	if !ok {
		t.Fatal("SubjectFrom ok = false, want true")
	}
	if got.ID != "" || got.Roles != nil || got.Attributes != nil {
		t.Fatalf("zero subject roundtrip = %+v, want zero", got)
	}
}
