package resolver

import (
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
)

// TestResolveDocCommentSurvivesIntoIR confirms ast.*Decl.DocComment set by
// the parser survives resolution onto the matching ir.* type, for every
// decl kind that carries one.
func TestResolveDocCommentSurvivesIntoIR(t *testing.T) {
	entity := entityWithIDField("User", stringField("name"))
	entity.DocComment = "The user of the system."
	entity.Fields[1].DocComment = "The user's display name."

	msg := &ast.MessageDecl{
		Name:       "Greeting",
		DocComment: "A DTO.",
		Fields:     []*ast.FieldDecl{{Name: "text", Type: &ast.TypeExpr{Name: "string"}}},
	}

	svc := &ast.ServiceDecl{
		Name:       "UserService",
		DocComment: "Manages users.",
		RPCs: []*ast.RPCDecl{
			{
				Name:       "GetUser",
				Returns:    "User",
				DocComment: "Fetches one user.",
				Auth:       &ast.IdentValue{Name: "required"},
			},
		},
	}

	job := &ast.JobDecl{
		Name:       "SendWelcomeEmail",
		DocComment: "Sends a welcome email.",
	}

	sched := &ast.ScheduleDecl{
		Name:       "NightlyCleanup",
		Cron:       "0 0 * * *",
		DocComment: "Runs nightly cleanup.",
	}

	files := []*ast.File{{
		Name:  "schema/app.zen",
		Decls: []ast.Decl{entity, msg, svc, job, sched},
	}}

	schema, diags := Resolve(files)
	mustNotHaveErrors(t, diags)

	if len(schema.Modules) != 1 {
		t.Fatalf("Modules len = %d, want 1", len(schema.Modules))
	}

	module := schema.Modules[0]

	if len(module.Entities) != 1 || module.Entities[0].DocComment != "The user of the system." {
		t.Fatalf("Entity DocComment = %+v, want %q", module.Entities, "The user of the system.")
	}

	if got := module.Entities[0].FieldByName("name").DocComment; got != "The user's display name." {
		t.Errorf("Field(name).DocComment = %q, want %q", got, "The user's display name.")
	}

	if len(module.Messages) != 1 || module.Messages[0].DocComment != "A DTO." {
		t.Fatalf("Message DocComment = %+v, want %q", module.Messages, "A DTO.")
	}

	if len(module.Services) != 1 || module.Services[0].DocComment != "Manages users." {
		t.Fatalf("Service DocComment = %+v, want %q", module.Services, "Manages users.")
	}

	if len(module.Services[0].Operations) != 1 || module.Services[0].Operations[0].DocComment != "Fetches one user." {
		t.Fatalf("Operation DocComment = %+v, want %q", module.Services[0].Operations, "Fetches one user.")
	}

	if len(module.Jobs) != 1 || module.Jobs[0].DocComment != "Sends a welcome email." {
		t.Fatalf("Job DocComment = %+v, want %q", module.Jobs, "Sends a welcome email.")
	}

	if len(module.Schedules) != 1 || module.Schedules[0].DocComment != "Runs nightly cleanup." {
		t.Fatalf("Schedule DocComment = %+v, want %q", module.Schedules, "Runs nightly cleanup.")
	}
}
