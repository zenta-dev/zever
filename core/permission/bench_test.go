package permission

import (
	"testing"
)

// benchAdapter registers a stub checker once and returns its adapter so
// Open can be measured without paying the (one-shot) registration cost.
func benchAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (Checker, error) { return stubChecker{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkOpen(b *testing.B) {
	a := benchAdapter(b)
	opts := Options{Rules: []Rule{{Role: "admin", Action: "read", Effect: Allow}}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(a, opts); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := benchAdapter(b)
	opts := Options{Rules: []Rule{{Role: "admin", Action: "read", Effect: Allow}}}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, opts); err != nil {
				b.Errorf("Open(%v) error = %v", a, err)
				return
			}
		}
	})
}

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
