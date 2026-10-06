package session

import (
	"testing"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
)

// benchSessionAdapter registers a stub store once and returns its adapter so
// Open can be measured without the one-shot registration cost.
func benchSessionAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (Store, error) { return newStubStore(), nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkRegister(b *testing.B) {
	factory := func(Options) (Store, error) { return newStubStore(), nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := Register(freshAdapter(), factory); err != nil {
			b.Fatalf("Register() error = %v", err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	a := benchSessionAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenShared(b *testing.B) {
	a := freshAdapter()
	if err := RegisterShared(a, func(coredb.DB, Options) (Store, error) { return newStubStore(), nil }); err != nil {
		b.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := OpenShared(a, nil, Options{}); err != nil {
			b.Fatalf("OpenShared(%v) error = %v", a, err)
		}
	}
}

func BenchmarkNewID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if id := NewID(); len(id) != 64 {
			b.Fatalf("NewID len = %d, want 64", len(id))
		}
	}
}

func BenchmarkValidateID(b *testing.B) {
	id := NewID()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := ValidateID(id); err != nil {
			b.Fatalf("ValidateID(%q) error = %v", id, err)
		}
	}
}

func BenchmarkNewSession(b *testing.B) {
	id := NewID()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if s := NewSession(id, time.Hour); s.ID != id {
			b.Fatal("NewSession lost ID")
		}
	}
}

func BenchmarkSessionClone(b *testing.B) {
	s := NewSession(NewID(), time.Hour)
	s.Data["user"] = "alice"
	s.Data["roles"] = []string{"admin", "user"}
	s.Data["meta"] = map[string]any{"nested": map[string]any{"n": 1}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if c := s.Clone(); c.ID != s.ID {
			b.Fatal("Clone lost ID")
		}
	}
}
