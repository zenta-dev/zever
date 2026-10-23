package session

import (
	"testing"
	"time"

)

// benchSessionAdapter registers a stub store once and returns its adapter so
// Open can be measured without the one-shot registration cost.




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
