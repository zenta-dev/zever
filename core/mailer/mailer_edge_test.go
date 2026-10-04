package mailer

import (
	"errors"
	"strings"
	"testing"
)

func TestAddressValidate_exactLengthBoundaries(t *testing.T) {
	t.Parallel()

	maxAddr := strings.Repeat("a", 249) + "@b.co"
	if len(maxAddr) != 254 {
		t.Fatalf("test setup: addr len = %d, want 254", len(maxAddr))
	}
	if err := (Address{Address: maxAddr}).Validate(); err != nil {
		t.Fatalf("254-byte address err = %v, want nil", err)
	}

	maxName := strings.Repeat("n", 256)
	if err := (Address{Name: maxName, Address: "a@example.com"}).Validate(); err != nil {
		t.Fatalf("256-byte name err = %v, want nil", err)
	}
}

func TestAddressValidate_controlCharInName(t *testing.T) {
	t.Parallel()

	if err := (Address{Name: "bad\nname", Address: "a@example.com"}).Validate(); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("control char in name err = %v, want ErrInvalidAddress", err)
	}
}

func TestMailClone_preservesScalarsAndSlices(t *testing.T) {
	t.Parallel()

	orig := Mail{
		From:        Address{Name: "A", Address: "a@example.com"},
		To:          []Address{{Address: "b@example.com"}},
		Cc:          []Address{{Address: "c@example.com"}},
		Bcc:         []Address{{Address: "d@example.com"}},
		Subject:     "s",
		Body:        "b",
		HTML:        "<p>b</p>",
		Attachments: []Attachment{{Name: "f.txt", Content: []byte("x")}},
	}

	clone := orig.Clone()

	if clone.Subject != orig.Subject || clone.Body != orig.Body || clone.HTML != orig.HTML {
		t.Fatalf("Clone() scalar mismatch: %+v", clone)
	}
	if len(clone.To) != 1 || len(clone.Cc) != 1 || len(clone.Bcc) != 1 {
		t.Fatalf("Clone() slice lengths = %d/%d/%d, want 1/1/1", len(clone.To), len(clone.Cc), len(clone.Bcc))
	}

	clone.To[0].Address = "changed@example.com"
	if orig.To[0].Address != "b@example.com" {
		t.Fatal("Clone() aliased To slice")
	}
}
