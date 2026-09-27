package mailer

import (
	"errors"
	"strings"
	"testing"
)

func TestAddress_Validate_valid(t *testing.T) {
	t.Parallel()
	valid := []Address{
		{Address: "a@b.co"},
		{Name: "Jane", Address: "jane.doe+tag@example.com"},
		{Name: "A", Address: "x_y-z%1@example-domain.com"},
	}
	for _, a := range valid {
		if err := a.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v, want nil", a, err)
		}
	}
}

func TestAddress_Validate_missingAt(t *testing.T) {
	t.Parallel()
	a := Address{Address: "no-at-sign.example.com"}
	err := a.Validate()
	if !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err = %v, want ErrInvalidAddress", err)
	}
	var iae *InvalidAddressError
	if !errors.As(err, &iae) {
		t.Fatalf("err %T is not *InvalidAddressError", err)
	}
}

func TestAddress_Validate_doubleAt(t *testing.T) {
	t.Parallel()
	a := Address{Address: "a@@example.com"}
	if err := a.Validate(); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err = %v, want ErrInvalidAddress", err)
	}
}

func TestAddress_Validate_noDotInDomain(t *testing.T) {
	t.Parallel()
	a := Address{Address: "a@localhost"}
	if err := a.Validate(); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err = %v, want ErrInvalidAddress", err)
	}
}

func TestAddress_Validate_injectionAddr(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"a\r\nBcc: x@y.co", "a\n@example.com", "a@exa\rmple.com", "a b@example.com", "a(b)@example.com", "a<b>@example.com", "<a@example.com>", `"a"@example.com`} {
		a := Address{Address: raw}
		if err := a.Validate(); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("Validate(%q) = %v, want ErrInvalidAddress", raw, err)
		}
	}
}

func TestAddress_Validate_injectionName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Bad\r\nName", "Bad\nName", "Bad\rName", "Bad\x00Name"} {
		a := Address{Name: name, Address: "a@example.com"}
		if err := a.Validate(); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("Validate name %q = %v, want ErrInvalidAddress", name, err)
		}
	}
}

func TestAddress_Validate_overlong(t *testing.T) {
	t.Parallel()
	// addr > 254
	long := strings.Repeat("a", 250) + "@b.co"
	if err := (Address{Address: long}).Validate(); !errors.Is(err, ErrInvalidAddress) {
		t.Errorf("overlong addr err = nil, want ErrInvalidAddress")
	}
	// name > 256
	if err := (Address{Name: strings.Repeat("n", 257), Address: "a@example.com"}).Validate(); !errors.Is(err, ErrInvalidAddress) {
		t.Errorf("overlong name err = nil, want ErrInvalidAddress")
	}
	// empty local / domain
	for _, raw := range []string{"@example.com", "a@", "", "a@.com"} {
		if err := (Address{Address: raw}).Validate(); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("Validate(%q) = %v, want ErrInvalidAddress", raw, err)
		}
	}
}

func TestAddress_Validate_badChars(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"a!b@example.com", "a/b@example.com", "a?b@example.com", "a#b@example.com", "a@example_com.com", "a@exam ple.com", "a@exam,ple.com", "a@exam;ple.com", "a@exam:ple.com"} {
		if err := (Address{Address: raw}).Validate(); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("Validate(%q) = %v, want ErrInvalidAddress", raw, err)
		}
	}
}

func TestAddress_String_bareAndNamed(t *testing.T) {
	t.Parallel()
	if got := (Address{Address: "a@example.com"}).String(); got != "a@example.com" {
		t.Errorf("bare String() = %q", got)
	}
	if got := (Address{Name: "Jane", Address: "a@example.com"}).String(); got != "Jane <a@example.com>" {
		t.Errorf("named String() = %q", got)
	}
}

func TestAddress_String_nonASCII_qEncoded(t *testing.T) {
	t.Parallel()
	got := (Address{Name: "Jöhn", Address: "a@example.com"}).String()
	if !strings.Contains(got, "a@example.com") {
		t.Fatalf("String() = %q missing addr", got)
	}
	if !strings.Contains(got, "=?utf-8?q?") && !strings.Contains(got, "=?UTF-8?Q?") {
		t.Errorf("String() = %q want Q-encoded name", got)
	}
}
