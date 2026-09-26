package token

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/diag"
)

func TestKindString(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected string
	}{
		{"EOF", EOF, "EOF"},
		{"ILLEGAL", ILLEGAL, "ILLEGAL"},
		{"IDENT", IDENT, "IDENT"},
		{"INT", INT, "INT"},
		{"FLOAT", FLOAT, "FLOAT"},
		{"DURATION", DURATION, "DURATION"},
		{"STRING", STRING, "STRING"},
		{"LBRACE", LBRACE, "LBRACE"},
		{"RBRACE", RBRACE, "RBRACE"},
		{"LPAREN", LPAREN, "LPAREN"},
		{"RPAREN", RPAREN, "RPAREN"},
		{"LBRACK", LBRACK, "LBRACK"},
		{"RBRACK", RBRACK, "RBRACK"},
		{"COLON", COLON, "COLON"},
		{"COMMA", COMMA, "COMMA"},
		{"ARROW", ARROW, "ARROW"},
		{"AT", AT, "AT"},
		{"MINUS", MINUS, "MINUS"},
		{"QUESTION", QUESTION, "QUESTION"},
		{"ENTITY", ENTITY, "ENTITY"},
		{"SERVICE", SERVICE, "SERVICE"},
		{"JOB", JOB, "JOB"},
		{"SCHEDULE", SCHEDULE, "SCHEDULE"},
		{"RPC", RPC, "RPC"},
		{"INDEX", INDEX, "INDEX"},
		{"ENUM", ENUM, "ENUM"},
		{"HAS_MANY", HAS_MANY, "HAS_MANY"},
		{"HAS_ONE", HAS_ONE, "HAS_ONE"},
		{"BELONGS_TO", BELONGS_TO, "BELONGS_TO"},
		{"MANY_TO_MANY", MANY_TO_MANY, "MANY_TO_MANY"},
		{"MESSAGE", MESSAGE, "MESSAGE"},
		{"TRUE", TRUE, "TRUE"},
		{"FALSE", FALSE, "FALSE"},
		{"UnknownKind", Kind(9999), "ILLEGAL"}, // exhaustive switch default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.kind.String()
			if got != tt.expected {
				t.Fatalf("Kind.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestKeywords(t *testing.T) {
	tests := []struct {
		name        string
		keyword     string
		expected    Kind
		shouldExist bool
	}{
		{"entity", "entity", ENTITY, true},
		{"service", "service", SERVICE, true},
		{"job", "job", JOB, true},
		{"schedule", "schedule", SCHEDULE, true},
		{"rpc", "rpc", RPC, true},
		{"index", "index", INDEX, true},
		{"enum", "enum", ENUM, true},
		{"has_many", "has_many", HAS_MANY, true},
		{"has_one", "has_one", HAS_ONE, true},
		{"belongs_to", "belongs_to", BELONGS_TO, true},
		{"many_to_many", "many_to_many", MANY_TO_MANY, true},
		{"message", "message", MESSAGE, true},
		{"true", "true", TRUE, true},
		{"false", "false", FALSE, true},
		{"uuid (type name, not keyword)", "uuid", IDENT, false},
		{"string (type name, not keyword)", "string", IDENT, false},
		{"int64 (type name, not keyword)", "int64", IDENT, false},
		{"timestamp (type name, not keyword)", "timestamp", IDENT, false},
		{"http (contextual label, not keyword)", "http", IDENT, false},
		{"auth (contextual label, not keyword)", "auth", IDENT, false},
		{"permission (contextual label, not keyword)", "permission", IDENT, false},
		{"queue (contextual label, not keyword)", "queue", IDENT, false},
		{"retry (contextual label, not keyword)", "retry", IDENT, false},
		{"cron (contextual label, not keyword)", "cron", IDENT, false},
		{"dispatch (contextual label, not keyword)", "dispatch", IDENT, false},
		{"join_table (contextual label, not keyword)", "join_table", IDENT, false},
		{"GET (HTTP verb, not keyword)", "GET", IDENT, false},
		{"POST (HTTP verb, not keyword)", "POST", IDENT, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, exists := Keywords[tt.keyword]
			if !tt.shouldExist {
				if exists {
					t.Fatalf("Keywords[%q] = %v, want absent (should not be a keyword)", tt.keyword, got)
				}

				return
			}

			if !exists {
				t.Fatalf("Keywords[%q] not found, want %v", tt.keyword, tt.expected)
			}

			if got != tt.expected {
				t.Fatalf("Keywords[%q] = %v, want %v", tt.keyword, got, tt.expected)
			}
		})
	}
}

func TestToken(t *testing.T) {
	t.Run("Token creation", func(t *testing.T) {
		pos := diag.Position{File: "test.zen", Line: 1, Col: 5}
		tok := Token{
			Kind: IDENT,
			Lit:  "myvar",
			Pos:  pos,
		}

		if tok.Kind != IDENT {
			t.Fatalf("Token.Kind = %v, want %v", tok.Kind, IDENT)
		}

		if tok.Lit != "myvar" {
			t.Fatalf("Token.Lit = %q, want %q", tok.Lit, "myvar")
		}

		if tok.Pos.File != "test.zen" || tok.Pos.Line != 1 || tok.Pos.Col != 5 {
			t.Fatalf("Token.Pos = %v, want {File: test.zen, Line: 1, Col: 5}", tok.Pos)
		}
	})

	t.Run("String token with unescaped literal", func(t *testing.T) {
		pos := diag.Position{File: "test.zen", Line: 2, Col: 10}
		tok := Token{
			Kind: STRING,
			Lit:  "hello world",
			Pos:  pos,
		}

		if tok.Kind != STRING {
			t.Fatalf("Token.Kind = %v, want %v", tok.Kind, STRING)
		}

		if tok.Lit != "hello world" {
			t.Fatalf("Token.Lit = %q, want %q", tok.Lit, "hello world")
		}
	})
}
