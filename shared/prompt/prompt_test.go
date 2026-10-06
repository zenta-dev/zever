package prompt

import (
	"errors"
	"strings"
	"testing"
)

func TestSystemPrompt(t *testing.T) {
	t.Parallel()

	got := SystemPrompt("a reviewer", "Be terse.")

	if !strings.Contains(got, "You are a reviewer.") {
		t.Fatalf("missing role:\n%s", got)
	}

	if !strings.Contains(got, "Be terse.") {
		t.Fatalf("missing instructions:\n%s", got)
	}
}

func TestGroundContext(t *testing.T) {
	t.Parallel()

	got := GroundContext("What?", []string{"first", "second"})

	for _, want := range []string{"[1] first", "[2] second", "Question: What?"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestJSONRepair(t *testing.T) {
	t.Parallel()

	got := JSONRepair(`{"type":"object"}`, "{bad", errors.New("unexpected end"))

	for _, want := range []string{"not valid JSON", "unexpected end", "{bad"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestToolErrorFeedback(t *testing.T) {
	t.Parallel()

	got := ToolErrorFeedback("lookup", "timeout")

	for _, want := range []string{`"lookup"`, "timeout"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestSchemaDesign(t *testing.T) {
	t.Parallel()

	got := SchemaDesign("a blog")

	for _, want := range []string{"entity", "a blog"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestMigrationReview(t *testing.T) {
	t.Parallel()

	got := MigrationReview("drop users.email")

	for _, want := range []string{"breaking", "drop users.email"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestCitationCheck(t *testing.T) {
	t.Parallel()

	got := CitationCheck("What?", "Paris.", []string{"Paris is nice", "other"})

	for _, want := range []string{"[1] Paris is nice", "[2] other", "SUPPORTED"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestToolPlan(t *testing.T) {
	t.Parallel()

	got := ToolPlan("deploy", []string{"build", "push"})

	for _, want := range []string{"deploy", "- build", "- push"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorTriage(t *testing.T) {
	t.Parallel()

	got := DoctorTriage("auth: FAIL bad secret")

	for _, want := range []string{"doctor --json", "FAIL", "ZEVER_*"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestDiagRepair(t *testing.T) {
	t.Parallel()

	got := DiagRepair("app.zen:3:13: unknown field type")

	for _, want := range []string{"file:line:col", "app.zen:3:13"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestOutboxTriage(t *testing.T) {
	t.Parallel()

	got := OutboxTriage("dlq: 3 messages")

	for _, want := range []string{"requeue-vs-purge", "mask DSNs", "dlq: 3 messages"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestExtractSplit(t *testing.T) {
	t.Parallel()

	got := ExtractSplit("declares module billing alongside shipping")

	for _, want := range []string{"each module into its own file", "byte-exact", "billing alongside shipping"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}
