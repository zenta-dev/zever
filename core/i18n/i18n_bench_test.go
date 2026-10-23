package i18n

import (
	"testing"
)

func BenchmarkOptions_Validate(b *testing.B) {
	o := Options{Remote: RemoteOptions{Endpoint: "https://example.com/x", Timeout: 5e9, MaxInFlight: 100}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := o.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}
