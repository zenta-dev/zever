package cas_test

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/shared/cas"
)

// TestScripts_elseBranchReturnsZero verifies each script reports 0 when the
// owner check fails, so a stale holder never mutates a successor's lease.
func TestScripts_elseBranchReturnsZero(t *testing.T) {
	t.Parallel()

	for name, script := range map[string]string{
		"delete":  cas.CompareAndDeleteScript,
		"expire":  cas.CompareAndExpireScript,
		"persist": cas.CompareAndPersistScript,
	} {
		if !strings.Contains(script, "return 0") {
			t.Errorf("%s script missing zero-return else branch", name)
		}
	}
}

// TestExpireScript_usesTtlArgument verifies the expire script reads the TTL
// from ARGV[2] rather than a hardcoded value.
func TestExpireScript_usesTtlArgument(t *testing.T) {
	t.Parallel()

	if !strings.Contains(cas.CompareAndExpireScript, "ARGV[2]") {
		t.Error("expire script does not read TTL from ARGV[2]")
	}
}
