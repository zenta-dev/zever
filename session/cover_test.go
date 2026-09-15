package session

import (
	"errors"
	"strings"
	"testing"
)

// TestCoverTypedErrorStrings exercises the registry Error methods so
// message constructors stay covered. Prefix asserts only, per the
// no-brittle-assertions rule.
func TestCoverTypedErrorStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"duplicate", DuplicateError{Adapter: Memory}, "session: duplicate registration"},
		{"unknown", UnknownAdapterError{Adapter: Adapter(99)}, "session: unknown adapter"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.err.Error(); !strings.HasPrefix(got, tc.want) {
				t.Errorf("Error() = %q, want prefix %q", got, tc.want)
			}
		})
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("CSPRNG broken") }

func TestCoverNewIDCSPRNGFailurePanics(t *testing.T) {
	// Not parallel: swaps the package CSPRNG source.
	old := randReader
	randReader = errReader{}
	defer func() { randReader = old }()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewID with broken CSPRNG did not panic")
		}
	}()

	_ = NewID()
}

func TestCoverValidateIDCharset(t *testing.T) {
	t.Parallel()

	// 64 chars but non-hex: uppercase, 'g', and symbol each reject.
	for _, id := range []string{
		strings.Repeat("A", 64),
		strings.Repeat("g", 64),
		strings.Repeat("0", 63) + "!",
	} {
		if err := ValidateID(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("ValidateID(%q...) err = %v, want ErrInvalidID", id[:8], err)
		}
	}

	if err := ValidateID(strings.Repeat("ab12", 16)); err != nil {
		t.Errorf("ValidateID(64-hex) err = %v, want nil", err)
	}
}

func TestCoverCloneNestedTypes(t *testing.T) {
	t.Parallel()

	s := Session{Data: map[string]any{
		"m": map[string]string{"k": "v"},
		"s": []string{"a", "b"},
	}}
	c := s.Clone()

	cm, ok := c.Data["m"].(map[string]string)
	if !ok {
		t.Fatal("Clone lost map[string]string type")
	}
	cm["k"] = "changed"

	cs, ok := c.Data["s"].([]string)
	if !ok {
		t.Fatal("Clone lost []string type")
	}
	cs[0] = "changed"

	om, ok := s.Data["m"].(map[string]string)
	if !ok || om["k"] != "v" {
		t.Error("Clone shares map[string]string backing")
	}
	osl, ok := s.Data["s"].([]string)
	if !ok || osl[0] != "a" {
		t.Error("Clone shares []string backing")
	}
}
