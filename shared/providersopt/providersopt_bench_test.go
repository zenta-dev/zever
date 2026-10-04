package providersopt

import "testing"

// BenchmarkValidateEndpointValid measures validating a well-formed endpoint.
func BenchmarkValidateEndpointValid(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if errs := ValidateEndpoint("https://api.example.com/v1"); len(errs) != 0 {
			b.Fatalf("ValidateEndpoint() = %v", errs)
		}
	}
}

// BenchmarkValidateEndpointInvalid measures the rejection path.
func BenchmarkValidateEndpointInvalid(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ValidateEndpoint("example.com/v1")
	}
}

// BenchmarkPaddleEndpoint measures resolving the Paddle base URL.
func BenchmarkPaddleEndpoint(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = PaddleEndpoint("", true)
	}
}
