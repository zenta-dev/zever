package cas_test

import (
	"testing"

	"github.com/zenta-dev/zever/shared/cas"
)

// BenchmarkCompareAndDeleteDecision measures the compare decision the
// CompareAndDeleteScript encodes, as an offline reference for the atomic
// GET/DEL round trip the script performs server-side.
func BenchmarkCompareAndDeleteDecision(b *testing.B) {
	const (
		stored   = "holder-0123456789abcdef"
		expected = "holder-0123456789abcdef"
	)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = stored == expected
	}
}

// BenchmarkScriptSelection measures selecting a CAS script by name from the
// exported constants, the caller-side work before handing a script to Redis.
func BenchmarkScriptSelection(b *testing.B) {
	scripts := map[string]string{
		"delete":  cas.CompareAndDeleteScript,
		"expire":  cas.CompareAndExpireScript,
		"persist": cas.CompareAndPersistScript,
	}
	names := []string{"delete", "expire", "persist"}

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		_ = scripts[names[i%len(names)]]
		i++
	}
}
