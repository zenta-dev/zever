package naming

import (
	"testing"
)

func TestPascalCase(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// snake_case inputs
		{
			name: "snake_case to PascalCase",
			in:   "hello_world",
			want: "HelloWorld",
		},
		{
			name: "single word snake_case",
			in:   "hello",
			want: "Hello",
		},
		{
			name: "multiple words snake_case",
			in:   "foo_bar_baz",
			want: "FooBarBaz",
		},
		// already PascalCase inputs
		{
			name: "already PascalCase",
			in:   "HelloWorld",
			want: "HelloWorld",
		},
		{
			name: "single word PascalCase",
			in:   "Hello",
			want: "Hello",
		},
		// camelCase inputs
		{
			name: "camelCase to PascalCase",
			in:   "helloWorld",
			want: "HelloWorld",
		},
		// empty string
		{
			name: "empty string",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PascalCase(tt.in)
			if got != tt.want {
				t.Fatalf("PascalCase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestScreamingSnake(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// camelCase inputs
		{
			name: "camelCase to SCREAMING_SNAKE",
			in:   "helloWorld",
			want: "HELLO_WORLD",
		},
		{
			name: "single word camelCase",
			in:   "hello",
			want: "HELLO",
		},
		// PascalCase inputs
		{
			name: "PascalCase to SCREAMING_SNAKE",
			in:   "HelloWorld",
			want: "HELLO_WORLD",
		},
		{
			name: "single word PascalCase",
			in:   "Hello",
			want: "HELLO",
		},
		// snake_case inputs
		{
			name: "snake_case to SCREAMING_SNAKE",
			in:   "hello_world",
			want: "HELLO_WORLD",
		},
		{
			name: "single word snake_case",
			in:   "hello",
			want: "HELLO",
		},
		{
			name: "multiple underscores snake_case",
			in:   "foo_bar_baz",
			want: "FOO_BAR_BAZ",
		},
		// empty string
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		// acronym runs treated as one word, not split before every capital
		{
			name: "leading acronym",
			in:   "APIKey",
			want: "API_KEY",
		},
		{
			name: "leading acronym longer",
			in:   "HTTPServer",
			want: "HTTP_SERVER",
		},
		{
			name: "trailing acronym",
			in:   "AuthorID",
			want: "AUTHOR_ID",
		},
		{
			name: "bare acronym",
			in:   "ISBN",
			want: "ISBN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScreamingSnake(tt.in)
			if got != tt.want {
				t.Fatalf("ScreamingSnake(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSnakeCase(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "camelCase to snake_case",
			in:   "helloWorld",
			want: "hello_world",
		},
		{
			name: "PascalCase to snake_case",
			in:   "HelloWorld",
			want: "hello_world",
		},
		{
			name: "already snake_case unchanged",
			in:   "hello_world",
			want: "hello_world",
		},
		{
			name: "single word",
			in:   "hello",
			want: "hello",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "leading acronym",
			in:   "APIKey",
			want: "api_key",
		},
		{
			name: "longer leading acronym",
			in:   "HTTPServer",
			want: "http_server",
		},
		{
			name: "url acronym",
			in:   "URLConfig",
			want: "url_config",
		},
		{
			name: "trailing acronym",
			in:   "AuthorID",
			want: "author_id",
		},
		{
			name: "bare acronym",
			in:   "ISBN",
			want: "isbn",
		},
		{
			name: "non-acronym regression",
			in:   "BookAuthor",
			want: "book_author",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SnakeCase(tt.in)
			if got != tt.want {
				t.Fatalf("SnakeCase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPluralizeNaive(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// regular nouns (just add "s")
		{
			name: "regular noun dog",
			in:   "dog",
			want: "dogs",
		},
		{
			name: "regular noun cat",
			in:   "cat",
			want: "cats",
		},
		// "y" ending nouns (change "y" to "ies")
		{
			name: "y-ending noun baby",
			in:   "baby",
			want: "babies",
		},
		{
			name: "y-ending noun story",
			in:   "story",
			want: "stories",
		},
		// "s"/"x"/"ch"/"sh" ending nouns (add "es")
		{
			name: "s-ending noun class",
			in:   "class",
			want: "classes",
		},
		{
			name: "x-ending noun box",
			in:   "box",
			want: "boxes",
		},
		{
			name: "ch-ending noun church",
			in:   "church",
			want: "churches",
		},
		{
			name: "sh-ending noun dish",
			in:   "dish",
			want: "dishes",
		},
		// empty string
		{
			name: "empty string",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PluralizeNaive(tt.in)
			if got != tt.want {
				t.Fatalf("PluralizeNaive(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
