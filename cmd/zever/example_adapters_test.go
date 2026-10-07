package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// exampleAdapterDir maps a "<battery>/<adapter>" pair as written in a
// zever.yaml to the adapters/<battery>/<dir> package that implements it.
// Most pairs map one-to-one; the entries here cover adapters whose package
// directory name differs from the adapter selector.
var exampleAdapterDir = map[string]string{
	"password/argon2id":    "adapters/password/argon2",
	"search/sqlite":        "adapters/search/db",
	"search/db":            "adapters/search/db",
	"search/postgres":      "adapters/search/db",
	"vectorstore/sqlite":   "adapters/vectorstore/db",
	"vectorstore/db":       "adapters/vectorstore/db",
	"vectorstore/pgvector": "adapters/vectorstore/db",
	"workflow/postgres":    "adapters/workflow/db",
	"workflow/db":          "adapters/workflow/db",
	"resilience/memory":    "adapters/resilience/inproc",
}

// TestExampleAppsRegisterConfiguredAdapters is the guard that every example
// app registers each adapter its zever.yaml selects. Adapters never
// self-register, so an app that omits a Register call fails at startup with
// "unknown adapter"; this test catches that omission at build time instead.
func TestExampleAppsRegisterConfiguredAdapters(t *testing.T) {
	t.Parallel()

	root := repoRootAbs(t)

	yamls, err := filepath.Glob(filepath.Join(root, "examples", "*", "zever.yaml"))
	if err != nil {
		t.Fatalf("glob example configs: %v", err)
	}

	if len(yamls) == 0 {
		t.Fatal("no examples/*/zever.yaml found")
	}

	for _, yamlPath := range yamls {
		exampleDir := filepath.Dir(yamlPath)
		name := filepath.Base(exampleDir)

		raw, err := os.ReadFile(filepath.Join(exampleDir, "internal", "app", "app.go"))
		if err != nil {
			t.Logf("skip %s: no internal/app/app.go: %v", name, err)
			continue
		}

		app := string(raw)

		cfg, err := os.ReadFile(yamlPath)
		if err != nil {
			t.Fatalf("read %s: %v", yamlPath, err)
		}

		var doc map[string]struct {
			Adapter string `yaml:"adapter"`
		}

		if err := yaml.Unmarshal(cfg, &doc); err != nil {
			t.Fatalf("parse %s: %v", yamlPath, err)
		}

		for battery, svc := range doc {
			if svc.Adapter == "" {
				continue
			}

			key := battery + "/" + svc.Adapter
			dir, ok := exampleAdapterDir[key]
			if !ok {
				dir = "adapters/" + key
			}

			want := `"github.com/zenta-dev/zever/` + dir + `"`

			if !strings.Contains(app, want) {
				t.Errorf("%s: app.go does not register %s (missing import %s)", name, key, dir)
			}
		}
	}
}
