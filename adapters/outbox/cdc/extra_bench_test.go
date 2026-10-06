package cdc

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
)

func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{Options: outbox.Options{
		DSN:         "postgres://localhost/db",
		Slot:        "s",
		Publication: "p",
		Prefix:      "x",
	}}

	b.ReportAllocs()

	for b.Loop() {
		_ = opts.Validate()
	}
}

func BenchmarkSetRelayError(b *testing.B) {
	s := &store{}
	err := errors.New("boom")

	b.ReportAllocs()

	for b.Loop() {
		s.setRelayError(context.Background(), err)
	}
}
