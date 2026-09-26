package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// floorBundle lists the in-repo module roots every scaffolded app gets:
// container plus config, the core interfaces the generated entrypoints
// wire (log, router, job, authz, middleware), and the two stdlib-only
// adapters the floor registers (log/slog, router/stdhttp). It mirrors
// generate_server.go's coreBatteries plus the always-required container,
// config, core/job, core/authz and core/middleware modules renderNewGoMod
// pins.
func floorBundle() []string {
	return []string{
		"github.com/zenta-dev/zever/container",
		"github.com/zenta-dev/zever/config",
		"github.com/zenta-dev/zever/core/log",
		"github.com/zenta-dev/zever/core/router",
		"github.com/zenta-dev/zever/core/job",
		"github.com/zenta-dev/zever/core/authz",
		"github.com/zenta-dev/zever/core/middleware",
		"github.com/zenta-dev/zever/adapters/log/slog",
		"github.com/zenta-dev/zever/adapters/router/stdhttp",
	}
}

// heavySDKs lists third-party SDK import-path fragments no floor build may
// pull: one per nested heavy adapter module (go-redis, stripe, paddle,
// pgx, modernc, firebase, anthropic/openai/genai, meilisearch, qdrant,
// twilio, casbin, gofiber, posthog, googlemaps, chromedp, bild, argon2,
// oidc). Fragments are full module-path prefixes where possible so light
// lookalikes (stdlib maps, shared/redisopt helpers) never match.
func heavySDKs() []string {
	return []string{
		"github.com/redis/go-redis",
		"github.com/stripe/stripe-go",
		"github.com/PaddleHQ/paddle-go-sdk",
		"github.com/jackc/pgx",
		"modernc.org",
		"firebase.google.com/go",
		"github.com/anthropics/anthropic-sdk-go",
		"github.com/openai/openai-go",
		"google.golang.org/genai",
		"github.com/google/generative-ai-go",
		"github.com/meilisearch/meilisearch-go",
		"github.com/qdrant/go-client",
		"github.com/twilio/twilio-go",
		"github.com/casbin/casbin",
		"github.com/gofiber/fiber",
		"github.com/posthog/posthog-go",
		"googlemaps.github.io/maps",
		"github.com/chromedp/",
		"github.com/anthonynsimon/bild",
		"argon2",
		"github.com/coreos/go-oidc",
	}
}

// bundleDeps returns the closed dependency set of pkgs via go list -deps,
// run from the workspace root so every in-repo module resolves. It shells
// out instead of scaffolding a temp app: deterministic and offline.
func bundleDeps(t *testing.T, pkgs []string) map[string]bool {
	t.Helper()

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not on PATH")
	}

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.work")); statErr == nil {
			break
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.work not found walking up from test dir")
		}

		dir = parent
	}

	args := append([]string{"list", "-deps"}, pkgs...)
	cmd := exec.CommandContext(t.Context(), goBin, args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %v: %v", pkgs, err)
	}

	set := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			set[line] = true
		}
	}

	return set
}

func assertNoHeavy(t *testing.T, deps map[string]bool, except ...string) {
	t.Helper()

	skip := map[string]bool{}
	for _, e := range except {
		skip[e] = true
	}

	for dep := range deps {
		for _, heavy := range heavySDKs() {
			if skip[heavy] {
				continue
			}

			if strings.Contains(dep, heavy) {
				t.Errorf("floor bundle pulls heavy dep %q (matched %q)", dep, heavy)
			}
		}
	}
}

// TestBundleProof asserts the scaffold floor's dependency closure stays
// slim: the floor set pulls no third-party SDK, adding cache/redis pulls
// go-redis (and still not stripe), and adding db/postgres pulls pgx (and
// still not modernc).
func TestBundleProof(t *testing.T) {
	t.Parallel()

	t.Run("floor", func(t *testing.T) {
		t.Parallel()

		assertNoHeavy(t, bundleDeps(t, floorBundle()))
	})

	t.Run("redis", func(t *testing.T) {
		t.Parallel()

		pkgs := append(floorBundle(), "github.com/zenta-dev/zever/adapters/cache/redis")
		deps := bundleDeps(t, pkgs)

		found := false
		for dep := range deps {
			if strings.Contains(dep, "github.com/redis/go-redis") {
				found = true
				break
			}
		}

		if !found {
			t.Error("cache/redis bundle missing github.com/redis/go-redis")
		}

		for dep := range deps {
			if strings.Contains(dep, "github.com/stripe/stripe-go") {
				t.Errorf("cache/redis bundle unexpectedly pulls %q", dep)
			}
		}
	})

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()

		pkgs := append(floorBundle(), "github.com/zenta-dev/zever/adapters/db/postgres")
		deps := bundleDeps(t, pkgs)

		found := false
		for dep := range deps {
			if strings.Contains(dep, "github.com/jackc/pgx") {
				found = true
				break
			}
		}

		if !found {
			t.Error("db/postgres bundle missing github.com/jackc/pgx")
		}

		for dep := range deps {
			if strings.Contains(dep, "modernc.org") {
				t.Errorf("db/postgres bundle unexpectedly pulls %q", dep)
			}
		}
	})
}
