package sms_test

import (
	"encoding/json/jsontext"
	"testing"

	sms "github.com/example/zever-sms"

	"github.com/zenta-dev/zever/config"
)

// benchSMS returns a fresh stub backend constructed in code.
func benchSMS(b *testing.B) sms.SMS {
	b.Helper()

	svc, err := sms.New(sms.Options{From: "+15550000"})
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	return svc
}

// BenchmarkStubSend measures a stub Send round-trip.
func BenchmarkStubSend(b *testing.B) {
	svc := benchSMS(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := svc.Send(ctx, "+15550001", "hello"); err != nil {
			b.Fatalf("Send: %v", err)
		}
	}
}

// BenchmarkBuildFromConfig measures decoding cfg.Plugins["sms"] and
// constructing the code-wired backend: the container plugin build path.
func BenchmarkBuildFromConfig(b *testing.B) {
	cfg := config.Default()
	cfg.Plugins = map[string]config.Service[jsontext.Value]{
		"sms": {Options: jsontext.Value(`{"from":"+15550000"}`)},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := sms.BuildFromConfig(cfg); err != nil {
			b.Fatalf("BuildFromConfig: %v", err)
		}
	}
}

// BenchmarkNew measures direct backend construction in code.
func BenchmarkNew(b *testing.B) {
	opts := sms.Options{From: "+15550000"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := sms.New(opts); err != nil {
			b.Fatalf("New: %v", err)
		}
	}
}

// BenchmarkOptionsValidate measures options validation.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := sms.Options{From: "+15550000"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate: %v", err)
		}
	}
}
