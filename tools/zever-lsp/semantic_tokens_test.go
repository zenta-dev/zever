package main

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/parser"
)

// decodedToken is one delta-decoded semantic token, back in absolute
// coordinates. Tests assert against these rather than raw uint32 runs so a
// failure names a position instead of an array index.
type decodedToken struct {
	Line      int
	StartChar int
	Length    int
	TokenType string
}

// decodeSemanticTokens reverses encodeSemanticTokens, applying the same
// same-line/new-line rule in the opposite direction. It is an independent
// reimplementation on purpose: a round-trip through the encoder's own logic
// would pass even if that logic were wrong.
func decodeSemanticTokens(t *testing.T, data []uint32) []decodedToken {
	t.Helper()

	if len(data)%5 != 0 {
		t.Fatalf("semantic token data length %d is not a multiple of 5", len(data))
	}

	var (
		out      []decodedToken
		line     int
		startCol int
	)

	for i := 0; i < len(data); i += 5 {
		deltaLine := int(data[i])
		deltaChar := int(data[i+1])

		line += deltaLine
		if deltaLine == 0 {
			startCol += deltaChar
		} else {
			startCol = deltaChar
		}

		typeIndex := int(data[i+3])
		if typeIndex < 0 || typeIndex >= len(semanticTokenLegend.TokenTypes) {
			t.Fatalf("token type index %d is outside the legend", typeIndex)
		}

		out = append(out, decodedToken{
			Line:      line,
			StartChar: startCol,
			Length:    int(data[i+2]),
			TokenType: semanticTokenLegend.TokenTypes[typeIndex],
		})
	}

	return out
}

// classifySource parses src standalone and classifies it, returning the
// spans decoded back into absolute coordinates with named token types.
func classifySource(t *testing.T, src string) []decodedToken {
	t.Helper()

	file, _ := parser.New("semantic_test.zen", []byte(src)).ParseFile()
	if file == nil {
		t.Fatal("parser returned no file")
	}

	return decodeSemanticTokens(t, encodeSemanticTokens(classifyFile(file, src)))
}

// hasToken reports whether the decoded set contains exactly this span.
func hasToken(tokens []decodedToken, want decodedToken) bool {
	for _, tok := range tokens {
		if tok == want {
			return true
		}
	}

	return false
}

func TestSemanticTokenLegendIndicesAreResolved(t *testing.T) {
	// Every index the classifier can emit must exist in the legend; a -1
	// would silently drop those spans during encoding.
	indices := map[string]int{
		"namespace":  semTokNamespace,
		"type":       semTokType,
		"class":      semTokClass,
		"interface":  semTokInterface,
		"parameter":  semTokParameter,
		"property":   semTokProperty,
		"enumMember": semTokEnumMember,
		"event":      semTokEvent,
		"function":   semTokFunction,
		"method":     semTokMethod,
		"macro":      semTokMacro,
		"keyword":    semTokKeyword,
		"string":     semTokString,
		"number":     semTokNumber,
	}

	for name, index := range indices {
		if index < 0 {
			t.Errorf("token type %q is missing from the legend", name)
			continue
		}

		if got := semanticTokenLegend.TokenTypes[index]; got != name {
			t.Errorf("index %d resolves to %q, want %q", index, got, name)
		}
	}
}

// TestEncodeSemanticTokensDeltaEncodingAcrossLineBoundary pins the classic
// semantic-token off-by-one: once a token starts a new line, its character
// delta is the ABSOLUTE column, not a difference against the previous
// token's column on the previous line.
func TestEncodeSemanticTokensDeltaEncodingAcrossLineBoundary(t *testing.T) {
	spans := []tokenSpan{
		{Line: 0, StartChar: 20, Length: 4, TokenType: semTokKeyword},
		{Line: 3, StartChar: 2, Length: 5, TokenType: semTokProperty},
	}

	data := encodeSemanticTokens(spans)

	if len(data) != 10 {
		t.Fatalf("got %d encoded integers, want 10", len(data))
	}

	// First token: line 0, so deltaLine is 0 and deltaStartChar is absolute.
	if data[0] != 0 || data[1] != 20 {
		t.Errorf("first token = (deltaLine %d, deltaStartChar %d), want (0, 20)", data[0], data[1])
	}

	// Second token: three lines down, so deltaStartChar resets to its own
	// absolute column (2). The trap would produce 2-20 = -18, wrapping to a
	// huge uint32, or 18 if the difference were taken the other way round.
	if data[5] != 3 {
		t.Errorf("second token deltaLine = %d, want 3", data[5])
	}

	if data[6] != 2 {
		t.Errorf("second token deltaStartChar = %d, want 2 (absolute, not relative to the previous line)", data[6])
	}
}

// TestEncodeSemanticTokensSameLineUsesRelativeStartChar is the other half of
// the same rule: two tokens on one line encode the second's column as a
// difference against the first's.
func TestEncodeSemanticTokensSameLineUsesRelativeStartChar(t *testing.T) {
	spans := []tokenSpan{
		{Line: 5, StartChar: 4, Length: 2, TokenType: semTokProperty},
		{Line: 5, StartChar: 11, Length: 6, TokenType: semTokType},
	}

	data := encodeSemanticTokens(spans)

	if len(data) != 10 {
		t.Fatalf("got %d encoded integers, want 10", len(data))
	}

	if data[0] != 5 || data[1] != 4 {
		t.Errorf("first token = (deltaLine %d, deltaStartChar %d), want (5, 4)", data[0], data[1])
	}

	if data[5] != 0 {
		t.Errorf("second token deltaLine = %d, want 0 (same line)", data[5])
	}

	if data[6] != 7 {
		t.Errorf("second token deltaStartChar = %d, want 7 (11 - 4)", data[6])
	}
}

func TestEncodeSemanticTokensSortsAndDropsInvalidSpans(t *testing.T) {
	spans := []tokenSpan{
		{Line: 2, StartChar: 0, Length: 3, TokenType: semTokClass},
		{Line: 0, StartChar: 0, Length: 6, TokenType: semTokKeyword},
		{Line: 1, StartChar: 0, Length: 0, TokenType: semTokClass},     // zero length
		{Line: 1, StartChar: 0, Length: 4, TokenType: -1},              // unknown type
		{Line: 1, StartChar: -3, Length: 4, TokenType: semTokProperty}, // negative column
	}

	data := encodeSemanticTokens(spans)

	tokens := decodeSemanticTokens(t, data)
	if len(tokens) != 2 {
		t.Fatalf("got %d tokens, want 2 (invalid spans dropped)", len(tokens))
	}

	if tokens[0].Line != 0 || tokens[1].Line != 2 {
		t.Errorf("tokens are not in ascending line order: %+v", tokens)
	}
}

func TestEncodeSemanticTokensEmptyInput(t *testing.T) {
	if data := encodeSemanticTokens(nil); len(data) != 0 {
		t.Errorf("got %d integers for no spans, want 0", len(data))
	}
}

func TestEncodeSemanticTokensDoesNotReorderCallerSlice(t *testing.T) {
	spans := []tokenSpan{
		{Line: 4, StartChar: 0, Length: 1, TokenType: semTokKeyword},
		{Line: 1, StartChar: 0, Length: 1, TokenType: semTokKeyword},
	}

	encodeSemanticTokens(spans)

	if spans[0].Line != 4 {
		t.Errorf("caller slice was reordered: %+v", spans)
	}
}

// TestClassifyFileTagsEntityFieldAndAttributeSpans walks a small real schema
// and asserts each identifier class lands where it should. Positions are
// spelled out so a regression names the exact drift.
func TestClassifyFileTagsEntityFieldAndAttributeSpans(t *testing.T) {
	// Line/column map (0-based, tab-indented body lines):
	//   0: entity User {
	//        `entity` at 0, `User` at 7
	//   1: \tid: uuid @primary
	//        `id` at 1, `uuid` at 5, `primary` at 11
	//   2: \temail: string
	//        `email` at 1, `string` at 8
	//   3: \thas_many tasks: Task
	//        `has_many` at 1, `tasks` at 10, `Task` at 17
	//   4: }
	src := "entity User {\n" +
		"\tid: uuid @primary\n" +
		"\temail: string\n" +
		"\thas_many tasks: Task\n" +
		"}\n"

	tokens := classifySource(t, src)

	want := []decodedToken{
		{Line: 0, StartChar: 0, Length: 6, TokenType: "keyword"},   // entity
		{Line: 0, StartChar: 7, Length: 4, TokenType: "class"},     // User
		{Line: 1, StartChar: 1, Length: 2, TokenType: "property"},  // id
		{Line: 1, StartChar: 5, Length: 4, TokenType: "type"},      // uuid
		{Line: 1, StartChar: 11, Length: 7, TokenType: "macro"},    // primary
		{Line: 2, StartChar: 1, Length: 5, TokenType: "property"},  // email
		{Line: 2, StartChar: 8, Length: 6, TokenType: "type"},      // string
		{Line: 3, StartChar: 1, Length: 8, TokenType: "keyword"},   // has_many
		{Line: 3, StartChar: 10, Length: 5, TokenType: "property"}, // tasks
		{Line: 3, StartChar: 17, Length: 4, TokenType: "class"},    // Task
	}

	for _, expect := range want {
		if !hasToken(tokens, expect) {
			t.Errorf("missing %s token at %d:%d (len %d); got %+v",
				expect.TokenType, expect.Line, expect.StartChar, expect.Length, tokens)
		}
	}
}

// TestClassifyFileTagsMessageNameAndFields guards against a regression
// where declList's switch had no case for *ast.MessageDecl, so a message's
// name and fields were never tagged at all -- no class token for the
// message name, no property tokens for its fields, no type token for a
// field referencing another message by name.
func TestClassifyFileTagsMessageNameAndFields(t *testing.T) {
	// Line/column map (0-based, tab-indented body lines):
	//   0: message Greeting {
	//        `message` at 0, `Greeting` at 8
	//   1: \ttext: string
	//        `text` at 1, `string` at 7
	//   2: \tinner: Greeting
	//        `inner` at 1, `Greeting` at 8
	//   3: }
	src := "message Greeting {\n" +
		"\ttext: string\n" +
		"\tinner: Greeting\n" +
		"}\n"

	tokens := classifySource(t, src)

	want := []decodedToken{
		{Line: 0, StartChar: 0, Length: 7, TokenType: "keyword"},  // message
		{Line: 0, StartChar: 8, Length: 8, TokenType: "class"},    // Greeting (decl name)
		{Line: 1, StartChar: 1, Length: 4, TokenType: "property"}, // text
		{Line: 1, StartChar: 7, Length: 6, TokenType: "type"},     // string
		{Line: 2, StartChar: 1, Length: 5, TokenType: "property"}, // inner
		{Line: 2, StartChar: 8, Length: 8, TokenType: "type"},     // Greeting (field ref)
	}

	for _, expect := range want {
		if !hasToken(tokens, expect) {
			t.Errorf("missing %s token at %d:%d (len %d); got %+v",
				expect.TokenType, expect.Line, expect.StartChar, expect.Length, tokens)
		}
	}
}

func TestClassifyFileTagsLiteralsAndServiceMembers(t *testing.T) {
	//   0: service TaskAPI {
	//   1: \trpc ListTasks(user_id: uuid) -> Task {
	//   2: \t\thttp: GET "/tasks"
	//   3: \t}
	//   4: }
	src := "service TaskAPI {\n" +
		"\trpc ListTasks(user_id: uuid) -> Task {\n" +
		"\t\thttp: GET \"/tasks\"\n" +
		"\t}\n" +
		"}\n"

	tokens := classifySource(t, src)

	want := []decodedToken{
		{Line: 0, StartChar: 8, Length: 7, TokenType: "interface"},  // TaskAPI
		{Line: 1, StartChar: 1, Length: 3, TokenType: "keyword"},    // rpc
		{Line: 1, StartChar: 5, Length: 9, TokenType: "method"},     // ListTasks
		{Line: 1, StartChar: 15, Length: 7, TokenType: "parameter"}, // user_id
		{Line: 1, StartChar: 24, Length: 4, TokenType: "type"},      // uuid
		{Line: 1, StartChar: 33, Length: 4, TokenType: "class"},     // Task
		{Line: 2, StartChar: 8, Length: 3, TokenType: "enumMember"}, // GET
		{Line: 2, StartChar: 12, Length: 8, TokenType: "string"},    // "/tasks" incl. quotes
	}

	for _, expect := range want {
		if !hasToken(tokens, expect) {
			t.Errorf("missing %s token at %d:%d (len %d); got %+v",
				expect.TokenType, expect.Line, expect.StartChar, expect.Length, tokens)
		}
	}
}

func TestClassifyFileTagsNumberLiterals(t *testing.T) {
	//   0: job SendEmail(to: string) {
	//   1: \tqueue: mail
	//   2: \tretry: 3, 30s
	//   3: }
	src := "job SendEmail(to: string) {\n" +
		"\tqueue: mail\n" +
		"\tretry: 3, 30s\n" +
		"}\n"

	tokens := classifySource(t, src)

	want := []decodedToken{
		{Line: 0, StartChar: 0, Length: 3, TokenType: "keyword"},    // job
		{Line: 0, StartChar: 4, Length: 9, TokenType: "function"},   // SendEmail
		{Line: 1, StartChar: 8, Length: 4, TokenType: "enumMember"}, // mail
		{Line: 2, StartChar: 8, Length: 1, TokenType: "number"},     // 3
		{Line: 2, StartChar: 11, Length: 3, TokenType: "number"},    // 30s
	}

	for _, expect := range want {
		if !hasToken(tokens, expect) {
			t.Errorf("missing %s token at %d:%d (len %d); got %+v",
				expect.TokenType, expect.Line, expect.StartChar, expect.Length, tokens)
		}
	}
}

// TestClassifyFileEnumTypeIsClaimedByTheASTTier proves the one place the two
// tiers overlap resolves the way it is documented to: `enum` is a reserved
// keyword, but as a field's type name the AST tier claims the span first and
// reports it as a type.
func TestClassifyFileEnumTypeIsClaimedByTheASTTier(t *testing.T) {
	src := "entity Task {\n" +
		"\tstatus: enum(open, done)\n" +
		"}\n"

	tokens := classifySource(t, src)

	want := []decodedToken{
		{Line: 1, StartChar: 9, Length: 4, TokenType: "type"},        // enum
		{Line: 1, StartChar: 14, Length: 4, TokenType: "enumMember"}, // open
		{Line: 1, StartChar: 20, Length: 4, TokenType: "enumMember"}, // done
	}

	for _, expect := range want {
		if !hasToken(tokens, expect) {
			t.Errorf("missing %s token at %d:%d (len %d); got %+v",
				expect.TokenType, expect.Line, expect.StartChar, expect.Length, tokens)
		}
	}

	// And nothing was emitted twice for the same start position.
	seen := make(map[[2]int]int)
	for _, tok := range tokens {
		seen[[2]int{tok.Line, tok.StartChar}]++
	}

	for pos, count := range seen {
		if count > 1 {
			t.Errorf("position %d:%d classified %d times, want 1", pos[0], pos[1], count)
		}
	}
}

// TestClassifyFileHasNoCommentTokens makes the permanent comment limitation a
// tested contract. The lexer's skipLineComment discards "//" runs without
// emitting a token or recording a position, so comment spans exist nowhere in
// the token stream or the AST and cannot be recovered. This asserts the
// absence rather than leaving it an undocumented gap.
func TestClassifyFileHasNoCommentTokens(t *testing.T) {
	//   0: // the user of the system
	//   1: entity User {
	//   2: \tid: uuid // primary key comment
	//   3: }
	src := "// the user of the system\n" +
		"entity User {\n" +
		"\tid: uuid // primary key comment\n" +
		"}\n"

	tokens := classifySource(t, src)

	lines := strings.Split(src, "\n")

	for _, tok := range tokens {
		if tok.Line >= len(lines) {
			t.Fatalf("token on line %d, but the fixture has %d lines", tok.Line, len(lines))
		}

		// A comment starts at "//" and runs to end of line, so any span
		// beginning at or after that offset would be comment text.
		if start := strings.Index(lines[tok.Line], "//"); start >= 0 && tok.StartChar >= start {
			t.Errorf("token %s at %d:%d falls inside the comment starting at column %d",
				tok.TokenType, tok.Line, tok.StartChar, start)
		}
	}

	// The whole-line comment on line 0 must contribute nothing at all.
	for _, tok := range tokens {
		if tok.Line == 0 {
			t.Errorf("comment-only line 0 produced a %s token at column %d", tok.TokenType, tok.StartChar)
		}
	}

	// Sanity check: the fixture does produce tokens, so the assertions above
	// are not vacuously passing on an empty result.
	if len(tokens) == 0 {
		t.Fatal("fixture produced no tokens at all, so the comment assertions prove nothing")
	}
}

func TestClassifyFileHandlesNilFile(t *testing.T) {
	if spans := classifyFile(nil, ""); len(spans) != 0 {
		t.Errorf("got %d spans for a nil file, want 0", len(spans))
	}
}

func TestClassifyFileClassifiesSchedule(t *testing.T) {
	//   0: schedule Nightly {
	//   1: \tcron: "0 0 * * *"
	//   2: \tdispatch: SendEmail(to: "a@b.c")
	//   3: }
	src := "schedule Nightly {\n" +
		"\tcron: \"0 0 * * *\"\n" +
		"\tdispatch: SendEmail(to: \"a@b.c\")\n" +
		"}\n"

	tokens := classifySource(t, src)

	want := []decodedToken{
		{Line: 0, StartChar: 9, Length: 7, TokenType: "event"},      // Nightly
		{Line: 1, StartChar: 7, Length: 11, TokenType: "string"},    // "0 0 * * *"
		{Line: 2, StartChar: 11, Length: 9, TokenType: "function"},  // SendEmail
		{Line: 2, StartChar: 21, Length: 2, TokenType: "parameter"}, // to
		{Line: 2, StartChar: 25, Length: 7, TokenType: "string"},    // "a@b.c"
	}

	for _, expect := range want {
		if !hasToken(tokens, expect) {
			t.Errorf("missing %s token at %d:%d (len %d); got %+v",
				expect.TokenType, expect.Line, expect.StartChar, expect.Length, tokens)
		}
	}
}

// TestStringSpanLengthCoversEscapesAndNonASCII proves a string literal is
// measured over its real source extent, which Token.Lit cannot supply: it
// holds the unescaped value, so `"a\nb"` would otherwise be measured as 3.
func TestStringSpanLengthCoversEscapesAndNonASCII(t *testing.T) {
	tests := []struct {
		name string
		//nolint:lll // one literal per case reads better than a wrapped table
		src  string
		want int
	}{
		{"plain", "schedule S {\n\tcron: \"abc\"\n}\n", 5},
		// Source text is "a\"b": six characters including both delimiters.
		{"escaped quote", "schedule S {\n\tcron: \"a\\\"b\"\n}\n", 6},
		{"escaped newline", "schedule S {\n\tcron: \"a\\nb\"\n}\n", 6},
		{"non-ascii", "schedule S {\n\tcron: \"héllo\"\n}\n", 7},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokens := classifySource(t, tc.src)

			found := false

			for _, tok := range tokens {
				if tok.TokenType != "string" {
					continue
				}

				found = true

				if tok.Length != tc.want {
					t.Errorf("string span length = %d, want %d", tok.Length, tc.want)
				}
			}

			if !found {
				t.Fatal("no string token was produced")
			}
		})
	}
}

func TestUTF16Len(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"uuid", 4},
		{"héllo", 5},
		{"\"🙂\"", 4}, // a surrogate pair counts as two units, plus two quotes
	}

	for _, tc := range tests {
		if got := utf16Len(tc.in); got != tc.want {
			t.Errorf("utf16Len(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestEncodeSemanticTokensResultFitsSemanticTokensData proves the encoder
// output drops directly into the protocol's SemanticTokens.Data field, the
// shape the textDocument/semanticTokens/full handler returns.
func TestEncodeSemanticTokensResultFitsSemanticTokensData(t *testing.T) {
	spans := []tokenSpan{
		{Line: 0, StartChar: 0, Length: 6, TokenType: semTokKeyword},
		{Line: 0, StartChar: 7, Length: 4, TokenType: semTokClass},
	}

	result := protocol.SemanticTokens{Data: encodeSemanticTokens(spans)}

	if len(result.Data) != 10 {
		t.Fatalf("SemanticTokens.Data holds %d integers, want 10", len(result.Data))
	}

	tokens := decodeSemanticTokens(t, result.Data)
	if len(tokens) != 2 {
		t.Fatalf("decoded %d tokens, want 2", len(tokens))
	}
}

func TestSemanticTokenIndexUnknownName(t *testing.T) {
	if got := semanticTokenIndex("not-a-token-type"); got != -1 {
		t.Errorf("semanticTokenIndex() = %d, want -1", got)
	}
}

func TestSpanCollectorAddSpanGuards(t *testing.T) {
	pos := diag.Position{Line: 1, Col: 1}

	tests := []struct {
		name      string
		pos       diag.Position
		length    int
		tokenType int
		wantSpans int
	}{
		{"zero line dropped", diag.Position{Line: 0, Col: 1}, 3, semTokKeyword, 0},
		{"zero column dropped", diag.Position{Line: 1, Col: 0}, 3, semTokKeyword, 0},
		{"zero length dropped", pos, 0, semTokKeyword, 0},
		{"unknown type dropped", pos, 3, -1, 0},
		{"valid kept", pos, 3, semTokKeyword, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &spanCollector{taken: make(map[[2]int]bool)}
			c.addSpan(tc.pos, tc.length, tc.tokenType)

			if len(c.spans) != tc.wantSpans {
				t.Errorf("addSpan() produced %d spans, want %d", len(c.spans), tc.wantSpans)
			}
		})
	}
}

func TestSpanCollectorDuplicatePositionKeptOnce(t *testing.T) {
	c := &spanCollector{taken: make(map[[2]int]bool)}
	pos := diag.Position{Line: 1, Col: 1}

	c.addSpan(pos, 3, semTokKeyword)
	c.addSpan(pos, 3, semTokType)

	if len(c.spans) != 1 {
		t.Fatalf("addSpan() produced %d spans, want 1", len(c.spans))
	}

	if c.spans[0].TokenType != semTokKeyword {
		t.Errorf("kept TokenType = %d, want first-writer %d", c.spans[0].TokenType, semTokKeyword)
	}
}

// TestClassifyNilDecls proves every classifier tolerates the nil nodes a
// recovered parse can leave behind, producing no spans instead of crashing.
func TestClassifyNilDecls(t *testing.T) {
	c := &spanCollector{taken: make(map[[2]int]bool)}

	c.entity(nil)
	c.message(nil)
	c.relation(nil)
	c.index(nil)
	c.typeExpr(nil)
	c.service(nil)
	c.rpc(nil)
	c.param(nil)
	c.job(nil)
	c.schedule(nil)
	c.value(nil)
	c.entity(&ast.EntityDecl{Fields: []*ast.FieldDecl{nil}})
	c.message(&ast.MessageDecl{Fields: []*ast.FieldDecl{nil}})
	c.service(&ast.ServiceDecl{RPCs: []*ast.RPCDecl{nil}})
	c.job(&ast.JobDecl{Params: []*ast.ParamDecl{nil}})
	c.attributes([]*ast.Attribute{nil})
	c.args([]*ast.Arg{nil})

	if len(c.spans) != 0 {
		t.Errorf("nil declarations produced %d spans, want 0: %+v", len(c.spans), c.spans)
	}
}

func TestClassifyIndexDeclColumns(t *testing.T) {
	c := &spanCollector{taken: make(map[[2]int]bool)}

	// More columns than positions: the trailing column is dropped, not
	// indexed out of range.
	c.index(&ast.IndexDecl{
		Columns:   []string{"id", "email"},
		ColumnPos: []diag.Position{{Line: 2, Col: 8}},
	})

	if len(c.spans) != 1 {
		t.Fatalf("index() produced %d spans, want 1: %+v", len(c.spans), c.spans)
	}

	if c.spans[0].TokenType != semTokProperty {
		t.Errorf("index column TokenType = %d, want property %d", c.spans[0].TokenType, semTokProperty)
	}
}

func TestClassifyTypeExprArgPosShort(t *testing.T) {
	c := &spanCollector{taken: make(map[[2]int]bool)}

	c.typeExpr(&ast.TypeExpr{
		NamePos: diag.Position{Line: 1, Col: 9},
		Name:    "enum",
		Args:    []string{"open", "done"},
		ArgPos:  []diag.Position{{Line: 1, Col: 14}},
	})

	found := false

	for _, span := range c.spans {
		if span.TokenType == semTokEnumMember {
			found = true
		}
	}

	if !found {
		t.Errorf("typeExpr() dropped every enum member, want the positioned one: %+v", c.spans)
	}
}

func TestClassifyValueShapes(t *testing.T) {
	pos := diag.Position{Line: 1, Col: 1}

	t.Run("ident", func(t *testing.T) {
		c := &spanCollector{taken: make(map[[2]int]bool)}
		c.value(&ast.IdentValue{Pos: pos, Name: "mail"})

		if len(c.spans) != 1 || c.spans[0].TokenType != semTokEnumMember {
			t.Errorf("ident value spans = %+v, want one enumMember", c.spans)
		}
	})

	t.Run("call", func(t *testing.T) {
		c := &spanCollector{taken: make(map[[2]int]bool)}
		c.value(&ast.CallValue{
			NamePos: pos,
			Name:    "now",
			Args:    []*ast.Arg{{Name: "x", Pos: pos, Value: &ast.IdentValue{Pos: pos, Name: "y"}}},
		})

		if len(c.spans) == 0 {
			t.Errorf("call value produced no spans, want function and parameter: %+v", c.spans)
		}
	})

	t.Run("set", func(t *testing.T) {
		c := &spanCollector{taken: make(map[[2]int]bool)}
		c.value(&ast.SetLit{Items: []string{"a", "b"}, ItemPos: []diag.Position{pos}})

		if len(c.spans) != 1 {
			t.Errorf("short-positioned set produced %d spans, want 1: %+v", len(c.spans), c.spans)
		}
	})
}

func TestClassifyPermissionResourceRef(t *testing.T) {
	src := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"}\n" +
		"service S {\n" +
		"\trpc M(id: uuid) -> Task {\n" +
		"\t\tauth: none\n" +
		"\t\tpermission: check(\"p\", resource: Task, owner_field: user_id)\n" +
		"\t}\n" +
		"}\n"

	tokens := classifySource(t, src)

	// Line 6 (0-based): `resource: Task` -- Task starts at column 35
	// ("\t\tpermission: check(\"p\", resource: ").
	want := decodedToken{Line: 6, StartChar: 35, Length: 4, TokenType: "class"}
	if !hasToken(tokens, want) {
		t.Errorf("missing class token for permission resource; got %+v", tokens)
	}
}

func TestClassifyJobRetryCall(t *testing.T) {
	src := "job SendEmail(to: string) {\n" +
		"\tqueue: mail\n" +
		"\tretry: max_attempts(3)\n" +
		"}\n"

	tokens := classifySource(t, src)

	want := []decodedToken{
		{Line: 2, StartChar: 8, Length: 12, TokenType: "function"}, // max_attempts
	}

	for _, expect := range want {
		if !hasToken(tokens, expect) {
			t.Errorf("missing %s token at %d:%d (len %d); got %+v",
				expect.TokenType, expect.Line, expect.StartChar, expect.Length, tokens)
		}
	}
}

func TestStringSpanLengthGuards(t *testing.T) {
	lines := splitLines([]byte("cron: \"abc\"\n"))

	if got := stringSpanLength([]byte("cron: \"abc\"\n"), lines, diag.Position{Line: 99, Col: 1}); got != 0 {
		t.Errorf("off-document line measured %d, want 0", got)
	}

	if got := stringSpanLength([]byte("cron: \"abc\"\n"), lines, diag.Position{Line: 1, Col: 1}); got != 0 {
		t.Errorf("non-quote start measured %d, want 0", got)
	}
}

func TestClassifyIndexDeclEndToEnd(t *testing.T) {
	//   3: \tindex(user_id, due_at)
	//        `index` at 1, `user_id` at 7, `due_at` at 16
	src := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tdue_at: timestamp\n" +
		"\tindex(user_id, due_at)\n" +
		"}\n"

	tokens := classifySource(t, src)

	want := []decodedToken{
		{Line: 4, StartChar: 1, Length: 5, TokenType: "keyword"},   // index
		{Line: 4, StartChar: 7, Length: 7, TokenType: "property"},  // user_id
		{Line: 4, StartChar: 16, Length: 6, TokenType: "property"}, // due_at
	}

	for _, expect := range want {
		if !hasToken(tokens, expect) {
			t.Errorf("missing %s token at %d:%d (len %d); got %+v",
				expect.TokenType, expect.Line, expect.StartChar, expect.Length, tokens)
		}
	}
}

func TestClassifyRelationJoinTable(t *testing.T) {
	c := &spanCollector{taken: make(map[[2]int]bool)}

	c.relation(&ast.RelationDecl{
		FieldNamePos: diag.Position{Line: 4, Col: 2},
		FieldName:    "tags",
		TargetPos:    diag.Position{Line: 4, Col: 18},
		Target:       "Tag",
		Join:         &ast.JoinBlock{TablePos: diag.Position{Line: 4, Col: 36}, Table: "tag_map"},
	})

	tokens := decodeSemanticTokens(t, encodeSemanticTokens(c.spans))

	want := decodedToken{Line: 3, StartChar: 35, Length: 7, TokenType: "class"}
	if !hasToken(tokens, want) {
		t.Errorf("missing class token for join table; got %+v", tokens)
	}
}

func TestStringSpanLengthStopsAtNewline(t *testing.T) {
	src := []byte("\"ab\ncd\"")
	lines := splitLines(src)

	// Quote, a, b: the scan stops at the newline of the unterminated
	// literal instead of running into the next line.
	if got := stringSpanLength(src, lines, diag.Position{Line: 1, Col: 1}); got != 3 {
		t.Errorf("unterminated literal measured %d, want 3", got)
	}
}
