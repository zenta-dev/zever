// Command apidiff gates Core-tier public API compatibility against the
// last release tag of each module.
//
// For every module in the Core tier it exports the package API at the
// module's latest <dir>/vX.Y.Z tag (via git archive) and at the working
// tree, compares the two with golang.org/x/exp/apidiff, and fails when an
// incompatible change is not documented by a **Breaking:** line under
// ## [Unreleased] in CHANGELOG.md.
//
// Usage: apidiff [--root DIR] [--modules a,b,c] [--changelog FILE]
// apidiff --check-patch [--changelog FILE] fails when a released patch
// section documents a breaking change.
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
