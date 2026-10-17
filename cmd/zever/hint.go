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

// unknownAdapterError reports an unknown adapter pick for a known battery:
// what (the bad adapter for this battery), why (typo or unsupported), and
// the exact next command (re-run with one of the listed choices).
func unknownAdapterError(tag, battery, input string, choices []string) error {
	msg := fmt.Sprintf("%s: unknown adapter %q for battery %q", tag, input, battery)

	if s := closest(input, choices); s != "" {
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
