package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// shouldShowHint reports whether hint footer should be emitted. Suppressed
// by ZEVER_NO_HINT or the global --quiet flag (quietMode in root.go).
func shouldShowHint() bool {
	return os.Getenv("ZEVER_NO_HINT") == "" && !quietMode
}

// hintedError carries a hint line under an error message: the failure
// first, the next step after it. It wraps the original error, so
// errors.Is and errors.As still reach it.
type hintedError struct {
	err  error
	hint string
}

// Error renders the wrapped error with the hint on the next line.
func (e *hintedError) Error() string { return e.err.Error() + "\n" + e.hint }

// Unwrap exposes the wrapped error for errors.Is and errors.As.
func (e *hintedError) Unwrap() error { return e.err }

// withHint attaches a hint line to err, or returns err unchanged when
// there is no hint to attach.
func withHint(err error, hint string) error {
	if err == nil || hint == "" {
		return err
	}

	return &hintedError{err: err, hint: hint}
}

// isTransientGoFailure reports whether go tool output looks like a flaky
// network/proxy error rather than a deterministic resolution or build error.
func isTransientGoFailure(out string) bool {
	for _, marker := range []string{
		"i/o timeout",
		"TLS handshake timeout",
		"connection reset by peer",
		"connection refused",
		"unexpected EOF",
		"no such host",
		"Client.Timeout exceeded",
		"503 Service Unavailable",
		"502 Bad Gateway",
		"504 Gateway Timeout",
		"429 Too Many Requests",
	} {
		if strings.Contains(out, marker) {
			return true
		}
	}

	return false
}

// unknownBatteryError reports an unknown battery name as what/why/next:
// what (the unknown name), why (a typo, or not a battery at all), and the
// exact next command (list batteries, then re-run with a real name). A
// did-you-mean suggestion rides along when one is close.
func unknownBatteryError(tag, input string) error {
	msg := fmt.Sprintf("%s: unknown battery %q", tag, input)

	if s := closest(input, sortedKeys(allServiceAdapters)); s != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", s)
	}

	msg += "; run `zever new --list-batteries` to list batteries"

	return errors.New(msg)
}

// removedAdapterAliases maps a removed adapter name to the canonical name
// that replaced it, per battery. The db-backed batteries (search,
// vectorstore, workflow) register one adapter each ("db"): the backend
// follows the DSN, so the per-backend names that only existed for the
// legacy file-based config format are gone. The map is not a compatibility
// shim -- a removed name never resolves -- it only lets a stale pick fail
// with a pointer to the replacement instead of a bare "unknown adapter".
var removedAdapterAliases = map[string]map[string]string{
	"search":      {"postgres": "db", "sqlite": "db"},
	"vectorstore": {"pgvector": "db", "sqlite": "db"},
	"workflow":    {"postgres": "db"},
}

// unknownAdapterError reports an unknown adapter pick for a known battery:
// what (the bad adapter for this battery), why (typo, unsupported, or a
// removed name), and the exact next command (re-run with one of the listed
// choices). A removed name points at the canonical replacement; anything
// else rides along with a did-you-mean suggestion when one is close.
func unknownAdapterError(tag, battery, input string, choices []string) error {
	msg := fmt.Sprintf("%s: unknown adapter %q for battery %q", tag, input, battery)

	if canonical, ok := removedAdapterAliases[battery][input]; ok {
		msg += fmt.Sprintf(" (removed name, use %q)", canonical)
	} else if s := closest(input, choices); s != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", s)
	}

	msg += "; choices: " + strings.Join(choices, ", ")

	return errors.New(msg)
}

// tidyFailureHint explains a `go mod tidy` failure as what/why/next: what
// failed, why it usually happens (offline module proxy, missing sums, or a
// toolchain too old for the go directive), and the exact command to re-run.
// out is the combined tidy output, dir the project directory.
func tidyFailureHint(out, dir string) string {
	if isGoTooOld(out) {
		return goTooOldHint(out)
	}

	if isTransientGoFailure(out) {
		return "the module proxy looks unreachable (transient network); check connectivity, then run `go mod tidy` in " + dir
	}

	return "run `go mod tidy` in " + dir + " manually to see the full error"
}

// goTooOldHint explains a go-toolchain-too-old failure as what/why/next:
// what (the toolchain predates the go directive), why (the scaffold targets
// a newer Go), and the exact next command (upgrade, then tidy).
func goTooOldHint(out string) string {
	return "go toolchain too old: " + firstGoTooOldLine(out) + "; install a newer Go (https://go.dev/doc/install), then run `go mod tidy`"
}

// isGoTooOld reports whether go tool output complains the toolchain is
// older than the module requires.
func isGoTooOld(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "requires go >=") || strings.Contains(lower, "go.mod requires go >=") {
			return true
		}
	}

	return false
}

// firstGoTooOldLine returns the first go-too-old complaint line, trimmed,
// for error text.
func firstGoTooOldLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if isGoTooOld(line) {
			return strings.TrimSpace(line)
		}
	}

	return strings.TrimSpace(out)
}

// generalTips rotates through contextual tips. Keep deterministic for tests.
var generalTips = []string{
	"run 'zever doctor' to verify batteries",
	"run 'zever dev' for watch-mode (recompile + restart on change)",
	"run 'zever generate --help' for scaffolding commands",
	"run 'zever compile --help' to see available backends",
}

// hintFor returns a contextual hint for a given successful command.
func hintFor(cmd string) string {
	switch cmd {
	case "new":
		return "next: cd <app> && zever compile --backend=zenorm,proto,atlas schema/app.zen && zever db migrate --adapter=sqlite --dsn=data/app.db schema/app.zen && zever serve"
	case "generate module":
		return "next: zever generate entity <module> <Name> --field title:string"
	case "generate entity":
		return "next: zever compile to validate schema"
	case "generate job":
		return "next: zever generate schedule <module> <Name> --cron \"*/5 * * * *\" --dispatch <Job>"
	case "generate schedule":
		return "next: verify with zever compile"
	case "generate server":
		return "next: zever serve  •  or zever dev for watch-mode"
	case "generate worker":
		return "next: zever queue:work  •  or zever dev"
	case "generate seed":
		return "next: zever db seed"
	case "generate tinker":
		return "next: zever tinker"
	case "generate adapter":
		return "next: implement TODOs in the new adapter package"
	case "extract":
		return "next: cd <service> && go mod tidy && zever compile"
	case "compile":
		return "next: zever db migrate --adapter=sqlite --dsn=data/app.db schema/*.zen"
	case "doctor":
		return "next: zever compile schema/app.zen"
	case "routes":
		return "tip: add http binding to an RPC to see more routes"
	case "check":
		return "next: zever compile schema/*.zen --backend=proto,zenorm"
	case "fmt":
		return "next: zever check schema/*.zen"
	case "check-boundaries", "check:boundaries":
		return "tip: keep modules isolated — move shared entities to their own module"
	case "db migrate":
		return "next: zever db seed  •  or zever serve"
	case "db seed":
		return "next: zever serve"
	default:
		return ""
	}
}
