package sms_test

import (
	"encoding/json"
	"testing"

	sms "github.com/example/zever-sms"
	"github.com/zenta-dev/zever/config"
)

// benchSMS returns an open stub backend after ensuring the plugin is
// registered exactly once.
func benchSMS(b *testing.B) sms.SMS {
	b.Helper()

	if err := ensureRegistered(); err != nil {
		b.Fatalf("RegisterPlugin: %v", err)
	}

	svc, err := sms.Open(sms.Stub, sms.Options{From: "+15550000"})
	if err != nil {
		b.Fatalf("Open: %v", err)
	}
	return svc
}

// BenchmarkStubSend measures a stub Send round-trip.
func BenchmarkStubSend(b *testing.B) {
	svc := benchSMS(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := svc.Send(ctx, "+15550001", "hello"); err != nil {
			b.Fatalf("Send: %v", err)
		}
	}
}

// BenchmarkBuildFromConfig measures decoding cfg.Plugins["sms"] and opening
// the selected adapter: the container plugin build path.
func BenchmarkBuildFromConfig(b *testing.B) {
	if err := ensureRegistered(); err != nil {
		b.Fatalf("RegisterPlugin: %v", err)
	}

	cfg := config.Default()
	cfg.Plugins = map[string]config.Service[json.RawMessage]{
		"sms": {Adapter: "stub", Options: json.RawMessage(`{"from":"+15550000"}`)},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sms.BuildFromConfig(cfg); err != nil {
			b.Fatalf("BuildFromConfig: %v", err)
		}
	}
}

// BenchmarkParseAdapter measures adapter-name parsing.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sms.ParseAdapter("stub"); err != nil {
			b.Fatalf("ParseAdapter: %v", err)
		}
	}
}

// BenchmarkOptionsValidate measures options validation.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := sms.Options{From: "+15550000"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate: %v", err)
		}
	}
}
