package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const seedUsageBody = `Scaffolds the database seed entrypoint at <seed_entry>/main.go (default
db/seed, override with project.seed_entry), plus internal/app/app.go if the
project does not have one yet.

The scaffold builds the container and opens the database; filling it is
project-specific, so the body is a worked comment showing how to call the
zen.InsertInto helpers against the zenorm backend's generated tables.`

// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// var seam keeps behavior identical while enabling success/error coverage.
var promptConfirmForSeed = promptConfirm

func printSeedUsage(fs *flag.FlagSet) {
	header := title("zever generate seed") + dim(" — scaffold seed entrypoint")
	usage := bold("Usage:") + "  " + cmd("zever generate seed") + dim(" [--force]") + dim("  •  -i/--interactive for guided prompts")

	printBoxedUsage(fs, header, usage, seedUsageBody, []usageExample{
		{command: "zever generate seed"},
		{command: "zever generate seed --force"},
		{command: "zever generate seed -i", comment: "  # confirm overwrite if exists"},
	}, "run with --force to re-scaffold")
}

// GenerateSeedConfig is the pure input to GenerateSeed: resolved project
// paths plus the force flag.
type GenerateSeedConfig struct {
	ModulePath   string
	SeedEntry    string
	GeneratedDir string
	Force        bool
	Stdout       io.Writer
	Stderr       io.Writer
}

// GenerateSeedResult names everything GenerateSeed wrote.
type GenerateSeedResult struct {
	Entrypoint string
	AppCreated bool
	Stub       string
	StubNew    bool
}

// GenerateSeed renders the seed entrypoint (and seed stub) from resolved
// inputs and writes them to disk. It performs no flag parsing and no
// prompting.
func GenerateSeed(cfg GenerateSeedConfig) (GenerateSeedResult, error) {
	const tag = "zever generate seed"

	var res GenerateSeedResult

	appCreated, err := ensureAppPackage(tag)
	if err != nil {
		return res, err
	}

	res.AppCreated = appCreated

	content, err := renderGoFile(tag, "seed main.go", seedTemplate, struct{ ModulePath string }{cfg.ModulePath})
	if err != nil {
		return res, err
	}

	path := filepath.Join(cfg.SeedEntry, "main.go")

	if writeErr := writeScaffold(tag, path, content, cfg.Force); writeErr != nil {
		return res, writeErr
	}

	res.Entrypoint = path

	stubContent, err := renderGoFile(tag, "seed stub", seedStubTemplate, struct{ GeneratedDir string }{cfg.GeneratedDir})
	if err != nil {
		return res, err
	}

	stubPath := filepath.Join(seedStubRoot, "seed.go")

	stubWritten, err := writeStubIfMissing(tag, stubPath, stubContent)
	if err != nil {
		return res, err
	}

	res.Stub = stubPath
	res.StubNew = stubWritten

	if stdout := cfg.Stdout; stdout != nil {
		if appCreated {
			_, _ = fmt.Fprintf(stdout, "%s %s\n", successMark(), success("scaffolded ")+cyan(filepath.Join("internal", "app", "app.go")))
		}

		_, _ = fmt.Fprintf(stdout, "%s %s %s\n", successMark(), success("scaffolded seed entrypoint at"), cyan(path))

		if stubWritten {
			_, _ = fmt.Fprintf(stdout, "%s %s %s\n", successMark(), success("scaffolded seed stub at"), cyan(stubPath))
		}
	}

	if stderr := cfg.Stderr; stderr != nil && shouldShowHint() {
		_, _ = fmt.Fprintln(stderr, formatHint("next: zever db seed or go run ./"+cfg.SeedEntry))
	}

	return res, nil
}

func runGenerateSeed(args []string) error {
	project, modulePath, forceVal, err := parseEntrypointFlags(args, "generate seed", printSeedUsage,
		func(p ProjectConfig) string { return p.SeedEntry }, promptConfirmForSeed)
	if err != nil {
		return err
	}

	_, err = GenerateSeed(GenerateSeedConfig{
		ModulePath:   modulePath,
		SeedEntry:    project.SeedEntry,
		GeneratedDir: project.GeneratedDir,
		Force:        forceVal,
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
	})

	return err
}

// seedStubRoot is the fixed directory the seed implementation is scaffolded
// under -- mirrors serverStubRoot/jobStubRoot: mechanical wiring (container
// setup, DB ping) stays in the always-regenerated db/seed/main.go, while the
// actual seed rows live here, once, never overwritten.
const seedStubRoot = "internal/service/seed"

// seedTemplate is the smallest useful seeder: container, database, context,
// shutdown — no HTTP and no queue wiring, since a seeder neither serves nor
// consumes anything. The actual seed rows live in seedStubTemplate's
// skip-if-exists output instead of inline here, so re-running
// `zever generate seed --force` (e.g. after project.seed_entry changes)
// never clobbers hand-written seed logic.
const seedTemplate = `// Command seed populates this project's database with development data.
//
// Scaffolded by ` + "`zever generate seed`" + `. Run it against a database that
// ` + "`zever db migrate`" + ` has already created the tables in.
package main

import (
	"context"
	"os"
	"time"

	"{{.ModulePath}}/internal/app"
	"{{.ModulePath}}/internal/service/seed"
)

const (
	shutdownGrace = 10 * time.Second
	seedTimeout   = 60 * time.Second
)

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString("seed: " + err.Error() + "\n")

		os.Exit(1)
	}
}

func run() error {
	c, err := app.New()
	if err != nil {
		return err
	}

	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()

		_ = c.Close(closeCtx)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout)
	defer cancel()

	logger, err := c.Log()
	if err != nil {
		return err
	}

	database, err := c.DB()
	if err != nil {
		return err
	}

	if err := database.Ping(ctx); err != nil {
		return err
	}

	if err := seed.Run(ctx, database); err != nil {
		return err
	}

	logger.Info().Msg("seed complete")

	return nil
}
`

// seedStubTemplate renders the skip-if-exists seed implementation: a worked
// example showing how to insert rows through zen against the zenorm backend's
// generated tables, written once and never regenerated.
const seedStubTemplate = `// Code generated by the zever DSL as a starting point.
// Fill in your real seed rows below -- this file is written once and never
// regenerated, so it is safe to edit freely.
package seed

import (
	"context"

	"github.com/zenta-dev/zever/db"
)

// Run seeds database with development data. Called once by db/seed/main.go
// after the database connection is confirmed reachable.
//
// TODO: insert your seed rows here, using the zen query builder against the
// tables the zenorm backend generates into {{.GeneratedDir}}/ from your .zen
// schema. With that package imported as orm, one row looks like:
//
//	if _, err := zen.InsertInto(orm.Users).Values(
//		zen.Set(orm.UserCols.ID, uuid.NewString()),
//		zen.Set(orm.UserCols.Email, "dev@example.com"),
//		zen.Set(orm.UserCols.CreatedAt, time.Now().UTC()),
//	).Exec(ctx, database); err != nil {
//		return err
//	}
//
// Seeding is normally idempotent: check for the row before inserting it, so
// re-running this command is safe.
func Run(ctx context.Context, database db.DB) error {
	return nil
}
`
