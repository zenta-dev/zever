package parser

import (
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
)

func TestParseEntityDocComment(t *testing.T) {
	src := "// The user of the system.\n" +
		"entity User {\n" +
		"  id: uuid @primary\n" +
		"}"

	file := mustParseClean(t, src)
	entity := firstEntity(t, file)

	if got, want := entity.DocComment, "The user of the system."; got != want {
		t.Errorf("EntityDecl.DocComment = %q, want %q", got, want)
	}
}

func TestParseEntityDocCommentMultiline(t *testing.T) {
	src := "// The user of the system.\n" +
		"// Owns tasks.\n" +
		"entity User {\n" +
		"  id: uuid @primary\n" +
		"}"

	file := mustParseClean(t, src)
	entity := firstEntity(t, file)

	want := "The user of the system.\nOwns tasks."
	if got := entity.DocComment; got != want {
		t.Errorf("EntityDecl.DocComment = %q, want %q", got, want)
	}
}

func TestParseEntityNoDocCommentWhenBlankLineBefore(t *testing.T) {
	src := "// unrelated comment\n" +
		"\n" +
		"entity User {\n" +
		"  id: uuid @primary\n" +
		"}"

	file := mustParseClean(t, src)
	entity := firstEntity(t, file)

	if entity.DocComment != "" {
		t.Errorf("EntityDecl.DocComment = %q, want empty (blank line breaks attachment)", entity.DocComment)
	}
}

func TestParseEntityNoDocCommentWhenTrailingOnPriorDecl(t *testing.T) {
	src := "entity Other {\n" +
		"  id: uuid @primary\n" +
		"} // trailing comment, not a doc comment\n" +
		"entity User {\n" +
		"  id: uuid @primary\n" +
		"}"

	file := mustParseClean(t, src)

	var user *ast.EntityDecl

	for _, d := range file.Decls {
		if e, ok := d.(*ast.EntityDecl); ok && e.Name == "User" {
			user = e
		}
	}

	if user == nil {
		t.Fatalf("entity User not found")
	}

	if user.DocComment != "" {
		t.Errorf("EntityDecl(User).DocComment = %q, want empty (prior line's comment is trailing, not standalone)", user.DocComment)
	}
}

func TestParseFieldDocComment(t *testing.T) {
	src := "entity User {\n" +
		"  // The primary key.\n" +
		"  id: uuid @primary\n" +
		"  // Not attached: separated by other content.\n" +
		"  name: string\n" +
		"}"

	file := mustParseClean(t, src)
	entity := firstEntity(t, file)

	if len(entity.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(entity.Fields))
	}

	if got, want := entity.Fields[0].DocComment, "The primary key."; got != want {
		t.Errorf("Fields[0].DocComment = %q, want %q", got, want)
	}

	if got, want := entity.Fields[1].DocComment, "Not attached: separated by other content."; got != want {
		t.Errorf("Fields[1].DocComment = %q, want %q", got, want)
	}
}

func TestParseMessageDocComment(t *testing.T) {
	src := "// A DTO.\n" +
		"message Greeting {\n" +
		"  text: string\n" +
		"}"

	file := mustParseClean(t, src)

	msg, ok := file.Decls[0].(*ast.MessageDecl)
	if !ok {
		t.Fatalf("expected *ast.MessageDecl, got %T", file.Decls[0])
	}

	if got, want := msg.DocComment, "A DTO."; got != want {
		t.Errorf("MessageDecl.DocComment = %q, want %q", got, want)
	}
}

func TestParseJobDocComment(t *testing.T) {
	src := "// Sends a welcome email.\n" +
		"job SendWelcomeEmail(user_id: uuid) {\n" +
		"  queue: default\n" +
		"}"

	file := mustParseClean(t, src)

	job, ok := file.Decls[0].(*ast.JobDecl)
	if !ok {
		t.Fatalf("expected *ast.JobDecl, got %T", file.Decls[0])
	}

	if got, want := job.DocComment, "Sends a welcome email."; got != want {
		t.Errorf("JobDecl.DocComment = %q, want %q", got, want)
	}
}

func TestParseScheduleDocComment(t *testing.T) {
	src := "// Runs nightly cleanup.\n" +
		"schedule NightlyCleanup {\n" +
		"  cron: \"0 0 * * *\"\n" +
		"}"

	file := mustParseClean(t, src)

	sched, ok := file.Decls[0].(*ast.ScheduleDecl)
	if !ok {
		t.Fatalf("expected *ast.ScheduleDecl, got %T", file.Decls[0])
	}

	if got, want := sched.DocComment, "Runs nightly cleanup."; got != want {
		t.Errorf("ScheduleDecl.DocComment = %q, want %q", got, want)
	}
}

func TestParseServiceAndRPCDocComment(t *testing.T) {
	src := "// Manages users.\n" +
		"service UserService {\n" +
		"  // Fetches one user.\n" +
		"  rpc GetUser(id: uuid) -> User {\n" +
		"  }\n" +
		"}"

	file := mustParseClean(t, src)

	svc, ok := file.Decls[0].(*ast.ServiceDecl)
	if !ok {
		t.Fatalf("expected *ast.ServiceDecl, got %T", file.Decls[0])
	}

	if got, want := svc.DocComment, "Manages users."; got != want {
		t.Errorf("ServiceDecl.DocComment = %q, want %q", got, want)
	}

	if len(svc.RPCs) != 1 {
		t.Fatalf("expected 1 rpc, got %d", len(svc.RPCs))
	}

	if got, want := svc.RPCs[0].DocComment, "Fetches one user."; got != want {
		t.Errorf("RPCDecl.DocComment = %q, want %q", got, want)
	}
}
