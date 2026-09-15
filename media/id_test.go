package media

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestValidID_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		id    string
		valid bool
	}{
		{"lower", "abc", true},
		{"upper", "ABC", true},
		{"digits", "0123456789", true},
		{"hyphen underscore", "a-1_b-2", true},
		{"single char", "x", true},
		{"empty", "", false},
		{"dot", ".", false},
		{"dotdot", "..", false},
		{"contains dotdot", "a..b", false},
		{"leading dotdot", "..a", false},
		{"trailing dotdot", "a..", false},
		{"single dot inside", "a.b", false},
		{"slash", "a/b", false},
		{"space", "a b", false},
		{"plus", "a+b", false},
		{"colon", "a:b", false},
		{"unicode", "h\u00e9llo", false},
		{"cjk", "\u753b\u50cf", false},
		{"emoji", "a\U0001F600b", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidID(tc.id); got != tc.valid {
				t.Errorf("ValidID(%q) = %v, want %v", tc.id, got, tc.valid)
			}
		})
	}
}

func TestValidHexID_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		id    string
		valid bool
	}{
		{"lower", "0123456789abcdef0123456789abcdef", true},
		{"upper", "0123456789ABCDEF0123456789ABCDEF", true},
		{"mixed", "aAbBcCdDeEfF00112233445566778899", true},
		{"empty", "", false},
		{"short", "abc", false},
		{"31 chars", strings.Repeat("a", 31), false},
		{"33 chars", strings.Repeat("a", 33), false},
		{"non-hex g", strings.Repeat("g", 32), false},
		{"dash", strings.Repeat("a", 31) + "-", false},
		{"space", strings.Repeat("a", 31) + " ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidHexID(tc.id); got != tc.valid {
				t.Errorf("ValidHexID(%q) = %v, want %v", tc.id, got, tc.valid)
			}
		})
	}
}

func TestGenerateID_unique_valid(t *testing.T) {
	t.Parallel()

	a, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID err = %v", err)
	}

	b, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID err = %v", err)
	}

	if a == b {
		t.Fatalf("GenerateID collision: %q", a)
	}

	if !ValidHexID(a) {
		t.Errorf("GenerateID %q is not a valid hex id", a)
	}

	if !ValidHexID(b) {
		t.Errorf("GenerateID %q is not a valid hex id", b)
	}
}

func TestGenerateIDWithReader_deterministic(t *testing.T) {
	t.Parallel()

	want := "000102030405060708090a0b0c0d0e0f"
	got, err := GenerateIDWithReader(bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}))
	if err != nil {
		t.Fatalf("GenerateIDWithReader err = %v", err)
	}

	if got != want {
		t.Errorf("GenerateIDWithReader = %q, want %q", got, want)
	}
}

type failReader struct {
	err error
}

func (r failReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func TestGenerateIDWithReader_failure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	got, err := GenerateIDWithReader(failReader{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Fatalf("GenerateIDWithReader err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "media: generate id") {
		t.Errorf("err %q missing %q", err.Error(), "media: generate id")
	}

	if got != "" {
		t.Errorf("GenerateIDWithReader value = %q, want empty", got)
	}
}

func TestGenerateIDWithReader_short(t *testing.T) {
	t.Parallel()

	if _, err := GenerateIDWithReader(bytes.NewReader([]byte{1, 2, 3})); err == nil {
		t.Errorf("GenerateIDWithReader short = nil, want error")
	}
}
