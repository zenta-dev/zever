package lexer

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/token"
)

// lexAllAndComments drains l via Next until EOF (dropping the EOF token, as
// LexAll does) and returns both the tokens and the CommentToken side
// channel.
func lexAllAndComments(l *Lexer) ([]token.Token, []CommentToken) {
	var toks []token.Token

	for {
		tok := l.Next()
		if tok.Kind == token.EOF {
			break
		}

		toks = append(toks, tok)
	}

	return toks, l.Comments()
}

func TestCommentsDoNotAppearInMainTokenStream(t *testing.T) {
	src := "// leading\n" +
		"entity User {\n" +
		"  id: uuid // trailing\n" +
		"}"

	l := New("test.zen", []byte(src))
	toks, comments := lexAllAndComments(l)

	for _, tok := range toks {
		if tok.Kind == token.ILLEGAL {
			t.Errorf("unexpected ILLEGAL token %+v -- comments must not leak into the main stream", tok)
		}
	}

	if len(comments) != 2 {
		t.Fatalf("expected 2 comments recorded, got %d: %+v", len(comments), comments)
	}
}

func TestStandaloneCommentAboveDecl(t *testing.T) {
	src := "// doc\n" +
		"entity User {\n" +
		"}"

	l := New("test.zen", []byte(src))
	_, comments := lexAllAndComments(l)

	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	if !comments[0].Standalone {
		t.Errorf("comment %+v: want Standalone == true", comments[0])
	}

	if comments[0].Text != "doc" {
		t.Errorf("comment text = %q, want %q", comments[0].Text, "doc")
	}

	if comments[0].Pos.Line != 1 {
		t.Errorf("comment line = %d, want 1", comments[0].Pos.Line)
	}
}

func TestTrailingCommentIsNotStandalone(t *testing.T) {
	src := "entity User { } // trailing\n"

	l := New("test.zen", []byte(src))
	_, comments := lexAllAndComments(l)

	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	if comments[0].Standalone {
		t.Errorf("comment %+v: want Standalone == false (shares a line with other tokens)", comments[0])
	}
}

func TestCommentAtEOFWithNoNewline(t *testing.T) {
	src := "entity User {} // no trailing newline"

	l := New("test.zen", []byte(src))
	_, comments := lexAllAndComments(l)

	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	if comments[0].Text != "no trailing newline" {
		t.Errorf("comment text = %q, want %q", comments[0].Text, "no trailing newline")
	}
}
