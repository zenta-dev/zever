package naming

import "testing"

func TestNamingEmptyStrings(t *testing.T) {
	t.Parallel()

	if got := PascalCase(""); got != "" {
		t.Fatalf("PascalCase(\"\") = %q, want \"\"", got)
	}

	if got := ScreamingSnake(""); got != "" {
		t.Fatalf("ScreamingSnake(\"\") = %q, want \"\"", got)
	}

	if got := SnakeCase(""); got != "" {
		t.Fatalf("SnakeCase(\"\") = %q, want \"\"", got)
	}

	if got := PluralizeNaive(""); got != "" {
		t.Fatalf("PluralizeNaive(\"\") = %q, want \"\"", got)
	}
}

func TestNamingSingleCharacters(t *testing.T) {
	t.Parallel()

	if got := PascalCase("a"); got != "A" {
		t.Fatalf("PascalCase(\"a\") = %q, want \"A\"", got)
	}

	if got := ScreamingSnake("A"); got != "A" {
		t.Fatalf("ScreamingSnake(\"A\") = %q, want \"A\"", got)
	}

	if got := SnakeCase("a"); got != "a" {
		t.Fatalf("SnakeCase(\"a\") = %q, want \"a\"", got)
	}
}

func TestNamingAcronymRuns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"APIKey", "api_key"},
		{"HTTPServer", "http_server"},
		{"getHTTPResponse", "get_http_response"},
		{"OAuth2Token", "o_auth2token"},
	}

	for _, tc := range tests {
		if got := SnakeCase(tc.in); got != tc.want {
			t.Fatalf("SnakeCase(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestScreamingSnakeFromSnake(t *testing.T) {
	t.Parallel()

	if got := ScreamingSnake("already_snake"); got != "ALREADY_SNAKE" {
		t.Fatalf("ScreamingSnake(\"already_snake\") = %q, want \"ALREADY_SNAKE\"", got)
	}

	if got := ScreamingSnake("Mixed_Case"); got != "MIXED_CASE" {
		t.Fatalf("ScreamingSnake(\"Mixed_Case\") = %q, want \"MIXED_CASE\"", got)
	}
}

func TestSnakeCaseLowercasesExistingSnake(t *testing.T) {
	t.Parallel()

	if got := SnakeCase("Already_Snake"); got != "already_snake" {
		t.Fatalf("SnakeCase(\"Already_Snake\") = %q, want \"already_snake\"", got)
	}
}

func TestPascalCaseSnakeWithEmptyParts(t *testing.T) {
	t.Parallel()

	if got := PascalCase("_private"); got != "Private" {
		t.Fatalf("PascalCase(\"_private\") = %q, want \"Private\"", got)
	}

	if got := PascalCase("trailing_"); got != "Trailing" {
		t.Fatalf("PascalCase(\"trailing_\") = %q, want \"Trailing\"", got)
	}
}

func TestPascalCaseDigits(t *testing.T) {
	t.Parallel()

	if got := PascalCase("v2_config"); got != "V2Config" {
		t.Fatalf("PascalCase(\"v2_config\") = %q, want \"V2Config\"", got)
	}
}

func TestPluralizeNaiveBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"category", "categories"},
		{"box", "boxes"},
		{"church", "churches"},
		{"dish", "dishes"},
		{"status", "statuses"},
		{"user", "users"},
		{"day", "daies"},
	}

	for _, tc := range tests {
		if got := PluralizeNaive(tc.in); got != tc.want {
			t.Fatalf("PluralizeNaive(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNamingDeterministic(t *testing.T) {
	t.Parallel()

	for i := 0; i < 2; i++ {
		if got := ScreamingSnake("userProfile"); got != "USER_PROFILE" {
			t.Fatalf("run %d: ScreamingSnake = %q, want \"USER_PROFILE\"", i, got)
		}
	}
}
