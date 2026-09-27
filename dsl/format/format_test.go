package format

import (
	"errors"
	"testing"
)

func TestFormatRoundTripsAlreadyCanonicalInput(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n" +
		"  id: uuid @primary\n" +
		"  email: string\n" +
		"}\n")

	out, err := Format(src)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if string(out) != string(src) {
		t.Fatalf("already-canonical input changed:\ngot:\n%q\nwant:\n%q", out, src)
	}
}

// TestFormatOptionalMarkHasNoLeadingSpace guards against a regression where
// CanonicalGap had no rule for token.QUESTION, so an optional field's `?`
// fell through to the " " default gap -- every `field: type?` was
// (incorrectly) rewritten with a space before the `?` on every format.
func TestFormatOptionalMarkHasNoLeadingSpace(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n" +
		"  id: uuid @primary\n" +
		"  image_id: uuid ?\n" +
		"  name: string ? @validate(max_len: 128)\n" +
		"}\n")

	want := "entity User {\n" +
		"  id: uuid @primary\n" +
		"  image_id: uuid?\n" +
		"  name: string? @validate(max_len: 128)\n" +
		"}\n"

	out, err := Format(src)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if string(out) != want {
		t.Fatalf("optional mark spacing not canonical:\ngot:\n%q\nwant:\n%q", out, want)
	}
}

func TestFormatFixesBadIndentation(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n" +
		"id: uuid @primary\n" +
		"        email: string\n" +
		"\t\t\tcreated_at: timestamp\n" +
		"  }\n")

	want := "entity User {\n" +
		"  id: uuid @primary\n" +
		"  email: string\n" +
		"  created_at: timestamp\n" +
		"}\n"

	out, err := Format(src)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if string(out) != want {
		t.Fatalf("indentation not canonical:\ngot:\n%q\nwant:\n%q", out, want)
	}
}

func TestFormatFixesSpacing(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n" +
		"\tid:uuid @ primary\n" +
		"\temail : string @validate(min_len : 1 , max_len:200)\n" +
		"}\n")

	want := "entity User {\n" +
		"  id: uuid @primary\n" +
		"  email: string @validate(min_len: 1, max_len: 200)\n" +
		"}\n"

	out, err := Format(src)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if string(out) != want {
		t.Fatalf("spacing not canonical:\ngot:\n%q\nwant:\n%q", out, want)
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n" +
		"id:uuid @ primary\n" +
		"        email : string\n" +
		"  }\n")

	once, err := Format(src)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	twice, err := Format(once)
	if err != nil {
		t.Fatalf("Format (second pass): %v", err)
	}

	if string(twice) != string(once) {
		t.Fatalf("format is not idempotent:\nfirst:\n%q\nsecond:\n%q", once, twice)
	}

	if edits, ok := Edits("", once); !ok {
		t.Fatalf("Edits reported dirty source on already-formatted input")
	} else if len(edits) != 0 {
		t.Fatalf("expected zero edits on already-formatted input, got %d: %+v", len(edits), edits)
	}
}

func TestFormatPreservesComments(t *testing.T) {
	t.Parallel()

	src := []byte("// leading\n" +
		"entity User {\n" +
		"id:uuid @ primary\n" +
		"   // inner note\n" +
		"  \n" +
		"        email : string\n" +
		"  }\n")

	out, err := Format(src)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	want := "// leading\n" +
		"entity User {\n" +
		"  id: uuid @primary\n" +
		"  // inner note\n" +
		"\n" +
		"  email: string\n" +
		"}\n"

	if string(out) != want {
		t.Fatalf("comments not preserved through formatting:\ngot:\n%q\nwant:\n%q", out, want)
	}
}

func TestFormatBailsOutOnSyntaxError(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n" +
		"name: string @default(\"unterminated)\n" +
		"}\n")

	_, err := Format(src)
	if err == nil {
		t.Fatalf("expected an error for source the lexer rejects, got nil")
	}

	if !errors.Is(err, ErrDirty) {
		t.Fatalf("expected ErrDirty, got %v", err)
	}
}

func TestEditsBailsOutOnIllegalCharacter(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n" +
		"id: uuid # nope\n" +
		"}\n")

	if _, ok := Edits("test.zen", src); ok {
		t.Fatalf("expected Edits to report dirty source for illegal input")
	}
}
