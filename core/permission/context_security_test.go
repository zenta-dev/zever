package permission

import (
	"testing"
)

func TestSubjectContextIsolation(t *testing.T) {
	t.Parallel()

	s := Subject{ID: "u1", Roles: []string{"admin"}, Attributes: map[string]string{"k": "v"}}
	ctx := WithSubject(t.Context(), s)

	// Mutating the source after storing must not affect the context.
	s.Roles[0] = "MUT"
	s.Attributes["k"] = "MUT"

	got, ok := SubjectFrom(ctx)
	if !ok {
		t.Fatal("SubjectFrom ok = false, want true")
	}
	if len(got.Roles) != 1 || got.Roles[0] != "admin" || got.Attributes["k"] != "v" {
		t.Fatalf("stored subject mutated via source: %+v", got)
	}

	// Mutating a lookup result must not affect later lookups.
	got.Roles[0] = "MUT2"
	got.Attributes["k"] = "MUT2"
	again, ok := SubjectFrom(ctx)
	if !ok {
		t.Fatal("SubjectFrom ok = false, want true")
	}
	if again.Roles[0] != "admin" || again.Attributes["k"] != "v" {
		t.Fatalf("stored subject mutated via lookup result: %+v", again)
	}
}
