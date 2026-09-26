// Package token defines lexical token kinds and keywords for the DSL compiler.
package token

import "github.com/zenta-dev/zever/dsl/diag"

// Kind represents a lexical token type.
type Kind int

const (
	// EOF marks the end of file.
	EOF Kind = iota
	// ILLEGAL represents an invalid token.
	ILLEGAL
	// IDENT represents an identifier.
	IDENT
	// INT represents an integer literal.
	INT
	// FLOAT represents a floating point literal.
	FLOAT
	// DURATION represents a duration literal.
	DURATION
	// STRING represents a string literal.
	STRING
	// LBRACE represents a left brace '{'.
	LBRACE
	// RBRACE represents a right brace '}'.
	RBRACE
	// LPAREN represents a left parenthesis '('.
	LPAREN
	// RPAREN represents a right parenthesis ')'.
	RPAREN
	// LBRACK represents a left bracket '['.
	LBRACK
	// RBRACK represents a right bracket ']'.
	RBRACK
	// COLON represents a colon ':'.
	COLON
	// COMMA represents a comma ','.
	COMMA
	// ARROW represents an arrow '->'.
	ARROW
	// AT represents an at sign '@'.
	AT
	// MINUS represents a minus sign '-'.
	MINUS
	// QUESTION represents a question mark '?'.
	QUESTION
	// ENTITY represents the "entity" keyword.
	ENTITY
	// SERVICE represents the "service" keyword.
	SERVICE
	// JOB represents the "job" keyword.
	JOB
	// SCHEDULE represents the "schedule" keyword.
	SCHEDULE
	// RPC represents the "rpc" keyword.
	RPC
	// INDEX represents the "index" keyword.
	INDEX
	// ENUM represents the "enum" keyword.
	ENUM
	// HAS_MANY represents the "has_many" keyword.
	//nolint:revive
	HAS_MANY
	// HAS_ONE represents the "has_one" keyword.
	//nolint:revive
	HAS_ONE
	// BELONGS_TO represents the "belongs_to" keyword.
	//nolint:revive
	BELONGS_TO
	// MANY_TO_MANY represents the "many_to_many" keyword.
	//nolint:revive
	MANY_TO_MANY
	// MESSAGE represents the "message" keyword.
	MESSAGE
	// TRUE represents the "true" keyword.
	TRUE
	// FALSE represents the "false" keyword.
	FALSE
)

// String returns the string representation of a Kind.
// Unknown kinds default to "ILLEGAL".
// nolint:gocyclo
func (k Kind) String() string {
	switch k {
	case EOF:
		return "EOF"
	case ILLEGAL:
		return "ILLEGAL"
	case IDENT:
		return "IDENT"
	case INT:
		return "INT"
	case FLOAT:
		return "FLOAT"
	case DURATION:
		return "DURATION"
	case STRING:
		return "STRING"
	case LBRACE:
		return "LBRACE"
	case RBRACE:
		return "RBRACE"
	case LPAREN:
		return "LPAREN"
	case RPAREN:
		return "RPAREN"
	case LBRACK:
		return "LBRACK"
	case RBRACK:
		return "RBRACK"
	case COLON:
		return "COLON"
	case COMMA:
		return "COMMA"
	case ARROW:
		return "ARROW"
	case AT:
		return "AT"
	case MINUS:
		return "MINUS"
	case QUESTION:
		return "QUESTION"
	case ENTITY:
		return "ENTITY"
	case SERVICE:
		return "SERVICE"
	case JOB:
		return "JOB"
	case SCHEDULE:
		return "SCHEDULE"
	case RPC:
		return "RPC"
	case INDEX:
		return "INDEX"
	case ENUM:
		return "ENUM"
	case HAS_MANY:
		return "HAS_MANY"
	case HAS_ONE:
		return "HAS_ONE"
	case BELONGS_TO:
		return "BELONGS_TO"
	case MANY_TO_MANY:
		return "MANY_TO_MANY"
	case MESSAGE:
		return "MESSAGE"
	case TRUE:
		return "TRUE"
	case FALSE:
		return "FALSE"
	default:
		return "ILLEGAL"
	}
}

// Keywords maps keyword strings to their Kind tokens.
// Note: type names (uuid, string, int64, timestamp, ...), contextual block labels
// (http, auth, permission, queue, retry, cron, dispatch, join_table), and HTTP verbs
// are NOT reserved keywords. Only the 12 words that unambiguously start a production
// wherever they appear are reserved.
var Keywords = map[string]Kind{
	"entity":       ENTITY,
	"service":      SERVICE,
	"job":          JOB,
	"schedule":     SCHEDULE,
	"rpc":          RPC,
	"index":        INDEX,
	"enum":         ENUM,
	"has_many":     HAS_MANY,
	"has_one":      HAS_ONE,
	"belongs_to":   BELONGS_TO,
	"many_to_many": MANY_TO_MANY,
	"message":      MESSAGE,
	"true":         TRUE,
	"false":        FALSE,
}

// Token represents a lexical token with its kind, literal text, and source position.
type Token struct {
	Kind Kind
	Lit  string // raw text for IDENT/INT/FLOAT/DURATION; unescaped for STRING
	Pos  diag.Position
}
