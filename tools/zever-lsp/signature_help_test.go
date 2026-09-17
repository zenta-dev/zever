package main

import (
	"context"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// posAfter returns the LSP position immediately after marker's last
// occurrence in src, assuming src is a single line. It fails the test if
// marker is not found.
func posAfter(t *testing.T, src, marker string) protocol.Position {
	t.Helper()

	idx := strings.LastIndex(src, marker)
	if idx < 0 {
		t.Fatalf("marker %q not found in %q", marker, src)
	}

	return protocol.Position{Character: uint32(idx + len(marker))} //nolint:gosec // indexes non-negative
}

// paramIndex looks a parameter's declared index up by name, so test
// expectations read as "the roles parameter" rather than a magic number
// that silently goes stale if signatureSpecs is reordered.
func paramIndex(t *testing.T, call, name string) int {
	t.Helper()

	spec, ok := signatureSpecs[call]
	if !ok {
		t.Fatalf("no signature spec for %q", call)
	}

	if i := paramIndexByName(spec.params, name); i >= 0 {
		return i
	}

	t.Fatalf("call %q has no parameter named %q", call, name)

	return -1
}

// activeParamOf extracts the active parameter index from signature help,
// failing the test when none is set.
func activeParamOf(t *testing.T, got *protocol.SignatureHelp) int {
	t.Helper()

	if got == nil {
		t.Fatal("signatureHelpAt returned nil")
	}

	active, ok := got.ActiveParameter.Get()
	if !ok {
		t.Fatal("ActiveParameter is absent, want a parameter index")
	}

	return int(active)
}

func TestSignatureHelpNoneOutsideCall(t *testing.T) {
	for _, src := range []string{
		"entity User {",
		"",
		"id: uuid",
		"foo(bar",
	} {
		got := signatureHelpAt(src, protocol.Position{Character: uint32(len(src))}) //nolint:gosec // len never negative
		if got != nil {
			t.Errorf("signatureHelpAt(%q) = %+v, want nil", src, got)
		}
	}
}

func TestSignatureHelpValidate(t *testing.T) {
	src := `@validate(min_len: 1, max_len: 128, format: "email")`

	tests := []struct {
		name   string
		marker string
		want   int
	}{
		{"open paren, no args yet", "@validate(", paramIndex(t, "validate", "format")},
		{"typing the name before its colon (positional fallback)", "min_len: 1, ", paramIndex(t, "validate", "min_len")},
		{"named arg fully typed", "max_len: ", paramIndex(t, "validate", "max_len")},
		{"third named arg fully typed", "format: ", paramIndex(t, "validate", "format")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cursor := posAfter(t, src, tc.marker)

			got := signatureHelpAt(src, cursor)
			if got == nil {
				t.Fatalf("signatureHelpAt returned nil at %q", tc.marker)
			}

			if len(got.Signatures) != 1 {
				t.Fatalf("Signatures = %d, want 1", len(got.Signatures))
			}

			wantLabel := "validate(format, min_len, max_len, gt, gte, lt, lte)"
			if got.Signatures[0].Label != wantLabel {
				t.Errorf("Label = %q, want %q", got.Signatures[0].Label, wantLabel)
			}

			if len(got.Signatures[0].Parameters) != 7 {
				t.Errorf("Parameters = %d, want 7", len(got.Signatures[0].Parameters))
			}

			if active := activeParamOf(t, got); active != tc.want {
				t.Errorf("ActiveParameter = %d, want %d", active, tc.want)
			}
		})
	}
}

func TestSignatureHelpRequiredRoles(t *testing.T) {
	src := `auth: required(roles: {owner, admin})`

	tests := []struct {
		name   string
		marker string
	}{
		{"open paren, no args yet", "required("},
		{"named arg fully typed", "roles: "},
		{"cursor inside the nested set literal", "roles: {owner, "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cursor := posAfter(t, src, tc.marker)

			got := signatureHelpAt(src, cursor)
			if got == nil {
				t.Fatalf("signatureHelpAt returned nil at %q", tc.marker)
			}

			wantLabel := "required(roles)"
			if got.Signatures[0].Label != wantLabel {
				t.Errorf("Label = %q, want %q", got.Signatures[0].Label, wantLabel)
			}

			// roles is the only parameter, and the nested set literal's comma
			// must not be mistaken for a top-level argument separator.
			if active := activeParamOf(t, got); active != 0 {
				t.Errorf("ActiveParameter = %d, want 0", active)
			}
		})
	}
}

func TestSignatureHelpPermissionCheck(t *testing.T) {
	src := `permission: check("policy-name", resource: User, owner_field: user_id)`

	tests := []struct {
		name   string
		marker string
		want   int
	}{
		{"open paren, no args yet", "check(", paramIndex(t, "check", "policy")},
		{"after positional string, before resource typed", `"policy-name", `, paramIndex(t, "check", "resource")},
		{"resource named arg fully typed", "resource: ", paramIndex(t, "check", "resource")},
		{"owner_field named arg fully typed", "owner_field: ", paramIndex(t, "check", "owner_field")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cursor := posAfter(t, src, tc.marker)

			got := signatureHelpAt(src, cursor)
			if got == nil {
				t.Fatalf("signatureHelpAt returned nil at %q", tc.marker)
			}

			if active := activeParamOf(t, got); active != tc.want {
				t.Errorf("ActiveParameter = %d, want %d", active, tc.want)
			}
		})
	}
}

func TestSignatureHelpJobRetry(t *testing.T) {
	t.Run("max_attempts", func(t *testing.T) {
		src := `retry: max_attempts(3)`
		cursor := posAfter(t, src, "max_attempts(")

		got := signatureHelpAt(src, cursor)
		if got == nil {
			t.Fatal("signatureHelpAt returned nil")
		}

		if got.Signatures[0].Label != "max_attempts(attempts)" {
			t.Errorf("Label = %q", got.Signatures[0].Label)
		}
	})

	t.Run("backoff positional kind then named base", func(t *testing.T) {
		src := `retry: backoff(exponential, base: 30s)`

		cursor := posAfter(t, src, "backoff(")

		got := signatureHelpAt(src, cursor)
		if got == nil {
			t.Fatal("signatureHelpAt returned nil")
		}

		if active := activeParamOf(t, got); active != 0 {
			t.Fatalf("at open paren: ActiveParameter = %d, want 0", active)
		}

		cursor = posAfter(t, src, "base: ")

		got = signatureHelpAt(src, cursor)
		if got == nil {
			t.Fatal("signatureHelpAt returned nil")
		}

		if active := activeParamOf(t, got); active != paramIndex(t, "backoff", "base") {
			t.Fatalf("at base:, ActiveParameter = %d", active)
		}
	})
}

func TestSignatureHelpFieldAttributes(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		src := `@default(now())`
		cursor := posAfter(t, src, "@default(")

		got := signatureHelpAt(src, cursor)
		if got == nil {
			t.Fatal("signatureHelpAt returned nil")
		}

		if got.Signatures[0].Label != "default(value)" {
			t.Errorf("Label = %q", got.Signatures[0].Label)
		}
	})

	t.Run("renamed_from", func(t *testing.T) {
		src := `@renamed_from("old_col")`
		cursor := posAfter(t, src, "@renamed_from(")

		got := signatureHelpAt(src, cursor)
		if got == nil {
			t.Fatal("signatureHelpAt returned nil")
		}

		if got.Signatures[0].Label != "renamed_from(old_name)" {
			t.Errorf("Label = %q", got.Signatures[0].Label)
		}
	})
}

// TestSignatureHelpParameterLabelsAreNativeArms proves the rendered
// signature uses the protocol's native union arms for parameter labels and
// documentation, not raw strings.
func TestSignatureHelpParameterLabelsAreNativeArms(t *testing.T) {
	src := `@validate(min_len: 1)`
	cursor := posAfter(t, src, "@validate(")

	got := signatureHelpAt(src, cursor)
	if got == nil {
		t.Fatal("signatureHelpAt returned nil")
	}

	for _, param := range got.Signatures[0].Parameters {
		if _, ok := param.Label.(protocol.String); !ok {
			t.Errorf("parameter Label = %T, want protocol.String", param.Label)
		}

		if _, ok := param.Documentation.(protocol.String); !ok {
			t.Errorf("parameter Documentation = %T, want protocol.String", param.Documentation)
		}
	}
}

// TestServerSignatureHelp exercises the textDocument/signatureHelp handler
// end to end through the server's workspace, not just the pure function.
func TestServerSignatureHelp(t *testing.T) {
	s := NewServer()

	rawURI := "file:///sig.zen"
	src := `entity User {
	email: string @validate(min_len: 1, )
}
`
	s.ws.SetDoc(rawURI, src)

	cursor := cursorOn(t, src, 1, ", )")
	cursor.Character += uint32(len(", "))

	got, err := s.signatureHelp(context.Background(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(rawURI)},
			Position:     cursor,
		},
	})
	if err != nil {
		t.Fatalf("signatureHelp error: %v", err)
	}

	if got == nil {
		t.Fatal("signatureHelp returned nil")
	}

	if got.Signatures[0].Label != "validate(format, min_len, max_len, gt, gte, lt, lte)" {
		t.Errorf("Label = %q", got.Signatures[0].Label)
	}

	if active := activeParamOf(t, got); active != paramIndex(t, "validate", "min_len") {
		t.Errorf("ActiveParameter = %d, want the position after min_len's comma", active)
	}
}

func TestServerSignatureHelpUnknownDocument(t *testing.T) {
	s := NewServer()

	got, err := s.signatureHelp(context.Background(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI("file:///missing.zen")},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("signatureHelp error: %v", err)
	}

	if got != nil {
		t.Errorf("signatureHelp() = %+v, want nil for an unknown document", got)
	}
}

func TestSignatureHelpOpenParenAlone(t *testing.T) {
	if got := signatureHelpAt("(x", protocol.Position{Character: 2}); got != nil {
		t.Errorf("signatureHelpAt() = %+v, want nil with no call name", got)
	}
}

func TestSignatureHelpNonWordBeforeParen(t *testing.T) {
	if got := signatureHelpAt("@(x", protocol.Position{Character: 3}); got != nil {
		t.Errorf("signatureHelpAt() = %+v, want nil when the paren follows no name", got)
	}
}

func TestSignatureHelpClampsPastLastParameter(t *testing.T) {
	src := `@validate(a, b, c, d, e, f, g, h`
	cursor := posAfter(t, src, "h")

	got := signatureHelpAt(src, cursor)
	if got == nil {
		t.Fatal("signatureHelpAt returned nil")
	}

	// Eight slots typed, seven parameters declared: the cursor pins to the
	// last one rather than running off the end.
	if active := activeParamOf(t, got); active != len(signatureSpecs["validate"].params)-1 {
		t.Errorf("ActiveParameter = %d, want the last parameter", active)
	}
}

func TestSignatureHelpNestedCallParensIgnored(t *testing.T) {
	src := `retry: backoff(warmup(1), base: 30s)`
	cursor := posAfter(t, src, "base: ")

	got := signatureHelpAt(src, cursor)
	if got == nil {
		t.Fatal("signatureHelpAt returned nil")
	}

	// The comma inside warmup(1) belongs to the nested call, so base: is
	// still the second top-level argument.
	if active := activeParamOf(t, got); active != paramIndex(t, "backoff", "base") {
		t.Errorf("ActiveParameter = %d, want the base parameter", active)
	}
}

func TestParamIndexByNameUnknown(t *testing.T) {
	if got := paramIndexByName(signatureSpecs["validate"].params, "bogus"); got != -1 {
		t.Errorf("paramIndexByName() = %d, want -1", got)
	}
}
