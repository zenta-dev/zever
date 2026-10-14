// Command vulngate enforces the govulncheck supply-chain gate: reachable
// vulnerabilities block, with an expiring allowlist for accepted risk.
//
// It runs `govulncheck -json ./...` in each given module directory, parses
// the finding stream, and fails when a reachable (called-symbol) finding is
// neither allowlisted nor expired. Unreachable (import-only) findings are
// reported but never block. Allowlist entries carry an expiry date; expired
// entries fail closed so accepted risk is re-triaged instead of rotting.
//
// Usage: vulngate [--root DIR] [--allowlist FILE] [--govulncheck BIN] dir...
package main

import (
	"context"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
