package local

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

// TestMain best-effort warms the OS page cache for every found Chrome/Chromium
// binary before tests run: each conformance subtest cold-launches headless
// Chrome via New(), and on loaded CI runners a cold launch can exceed the
// render budget. Running "--version" once per binary is deterministic (no
// sleeps); any error is ignored and tests run regardless. When no Chrome is
// found the render tests skip via hasChrome.
func TestMain(m *testing.M) {
	warmChromePageCache()
	os.Exit(m.Run())
}

func warmChromePageCache() {
	for _, name := range chromeBinaryNames {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}

		_ = exec.CommandContext(context.Background(), path, "--version").Run()
	}
}
