package permission

import (
	"context"
	"testing"
)

func TestSubject_roundtrip_present(t *testing.T) {
	t.Parallel()
	s := Subject{ID: "u1", Roles: []string{"admin"}, Attributes: map[string]string{"k": "v"}}
	ctx := WithSubject(context.Background(), s)
	got, ok := SubjectFrom(ctx)
	if !ok {
		t.Fatal("SubjectFrom ok = false, want true")
	}
	if got.ID != s.ID || len(got.Roles) != 1 || got.Attributes["k"] != "v" {
		t.Fatalf("roundtrip = %+v want %+v", got, s)
	}
}

func TestSubject_missing(t *testing.T) {
	t.Parallel()
	if _, ok := SubjectFrom(context.Background()); ok {
		t.Fatal("SubjectFrom ok = true, want false")
	}
}

func TestSubject_wrong_type(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), subjectKey{}, "not-a-subject")
	if _, ok := SubjectFrom(ctx); ok {
		t.Fatal("SubjectFrom ok = true for wrong type, want false")
	}
}
