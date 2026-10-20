package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShippedExamplesShareTheEnvHeader proves the shipped examples copy
// the header the generator writes: the key spelling rule (the option's
// serialized name, upper-cased) lives in that header, so a drifted example
// teaches a key spelling the resolver never matches.
func TestShippedExamplesShareTheEnvHeader(t *testing.T) {
	t.Parallel()

	root := repoRootAbs(t)

	examples := []string{"todo", "showcase", "bookings", "demoapp", "multidb"}

	for _, name := range examples {
		path := filepath.Join(root, "examples", name, ".env.example")

		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)

			continue
		}

		src := string(data)
		if !strings.HasPrefix(src, envExampleHeader) {
			t.Errorf("%s does not start with the generated header:\n%s", path, src)
		}
	}
}
