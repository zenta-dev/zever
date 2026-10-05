package shop

import (
	"context"
	"testing"
)

// TestContextWithSubjectRoundTrip pins the public hand-written-adapter
// entrypoint: a subject written with ContextWithSubject is what
// callerSubject reads back, without any authz claims present.
func TestContextWithSubjectRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := ContextWithSubject(context.Background(), "user-1")

	got, ok := callerSubject(ctx)
	if !ok || got != "user-1" {
		t.Fatalf("callerSubject = (%q, %v), want (\"user-1\", true)", got, ok)
	}
}

// TestContextWithSubjectEmptyIsAbsent proves an empty subject is treated as
// no identity rather than a present empty string.
func TestContextWithSubjectEmptyIsAbsent(t *testing.T) {
	t.Parallel()

	ctx := ContextWithSubject(context.Background(), "")

	if got, ok := callerSubject(ctx); ok {
		t.Fatalf("callerSubject = (%q, %v), want absent", got, ok)
	}
}

// TestCallerSubjectAbsentContext proves a context with neither a written
// subject nor authz claims reports no identity.
func TestCallerSubjectAbsentContext(t *testing.T) {
	t.Parallel()

	if got, ok := callerSubject(context.Background()); ok {
		t.Fatalf("callerSubject = (%q, %v), want absent", got, ok)
	}
}
