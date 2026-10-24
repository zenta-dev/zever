package permission

import (
	"testing"
)

// BenchmarkOptionsValidate measures Options validation for a permission backend.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{
		Rules: []Rule{
			{Role: "admin", Action: "read", Effect: Allow},
			{Role: "admin", Action: "delete", Effect: Deny},
			{Role: "user", Action: "read", OwnedOnly: true, OwnedAttr: "owner_id"},
		},
		Roles: map[string][]string{"admin": {"user", "ops"}, "user": {"guest"}},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

func BenchmarkRuleValidate(b *testing.B) {
	r := Rule{Role: "admin", Action: "read", Effect: Allow}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := r.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

func BenchmarkSubjectContext(b *testing.B) {
	s := Subject{ID: "u1", Roles: []string{"admin"}, Attributes: map[string]string{"k": "v"}}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		got, ok := SubjectFrom(WithSubject(ctx, s))
		if !ok || got.ID != s.ID {
			b.Fatal("SubjectFrom roundtrip failed")
		}
	}
}
