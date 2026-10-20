package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// usageHeader is the shared first line of the top-level help. printUsage
// (unknown-command path, main.go) renders it styled; newRootCmd (root.go)
// prepends it plain to the Cobra help template so `zever --help` and the
// unknown-command usage share the same header.
const usageHeader = "zever — zever toolkit"

// unknownSubcommandError builds the error for an unknown sub-subcommand
// (the second word of a two-word command): the error first, so it is the
// first line a developer reads, then the closest match as a hint, then the
// command that lists the real choices. It mirrors the top-level
// unknown-subcommand shape (see run in main.go): no usage dump in front of
// the error.
func unknownSubcommandError(command, sub string, choices []string, listCmd string) error {
	msg := fmt.Sprintf("%s: unknown subcommand %q", command, sub)

	if shouldShowHint() {
		if s := closest(sub, choices); s != "" {
			msg += "\n" + formatHint(fmt.Sprintf("did you mean %q?", s))
		}

		msg += "\n" + formatHint("run '"+listCmd+"' to list the subcommands")
	}

	return &subcommandError{msg: msg}
}

// subcommandError is the error an unrecognized sub-subcommand produces. It
// keeps the ErrUnknownSubcommand sentinel reachable through errors.Is, so
// a caller that branches on the sentinel still sees it.
type subcommandError struct {
	msg string
}

// Error renders the error message.
func (e *subcommandError) Error() string { return e.msg }

// Unwrap exposes ErrUnknownSubcommand for errors.Is.
func (e *subcommandError) Unwrap() error { return ErrUnknownSubcommand }

// allTopLevel returns every suggestible top-level name derived from the
// Cobra tree: each subcommand name plus each alias (including the deliberate
// check-boundaries/check:boundaries pair) plus help. Sorted for
// determinism; no hand-maintained list to drift.
func allTopLevel() []string {
	return topLevelNames(rootCmd)
}

// topLevelNames collects the suggestible names from root's subcommand
// tree. It takes the tree explicitly so tests can pass a fresh tree
// instead of the shared rootCmd global.
func topLevelNames(root *cobra.Command) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(n string) {
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	for _, c := range root.Commands() {
		add(c.Name())
		for _, a := range c.Aliases {
			add(a)
		}
	}
	// Cobra's default help command only materializes on Execute, so pin
	// the name for fresh trees.
	add("help")
	sort.Strings(out)
	return out
}

// generate subcommands.
var allGenerate = []string{
	"module", "entity", "job", "schedule", "server", "worker", "seed", "tinker", "adapter",
}

// db subcommands.
var allDB = []string{"migrate", "rollback", "seed"}

// backend names for compile.
var allBackends = []string{"proto", "zenorm", "atlas", "openapi", "gogen", "protogogen", "mcp"}

// closest returns the candidate with smallest edit distance to input, if distance
// is within a threshold (≤2, or ≤3 for longer strings). Case-insensitive.
func closest(input string, candidates []string) string {
	if len(candidates) == 0 || input == "" {
		return ""
	}

	lower := strings.ToLower(input)
	best := ""
	bestDist := 999

	// Scratch rows reused across candidates: three rows of max candidate width.
	maxLen := 0
	for _, c := range candidates {
		if len(c) > maxLen {
			maxLen = len(c)
		}
	}
	scratch := make([]int, 3*(maxLen+1))

	for _, c := range candidates {
		d := damerauLevenshteinScratch(lower, strings.ToLower(c), scratch)
		// Prefer prefix matches: distance bonus for prefix.
		if strings.HasPrefix(strings.ToLower(c), lower) && d > 1 {
			d--
		}

		if d < bestDist {
			bestDist = d
			best = c
		}
	}

	threshold := 2
	if len(input) > 6 {
		threshold = 3
	}
	// For very short inputs, require tighter threshold.
	if len(input) <= 3 && bestDist > 1 {
		return ""
	}

	if bestDist <= threshold {
		return best
	}

	return ""
}

// damerauLevenshtein computes edit distance with adjacent transposition.
func damerauLevenshtein(a, b string) int {
	return damerauLevenshteinScratch(a, b, nil)
}

// damerauLevenshteinScratch computes edit distance with adjacent transposition
// using three rolling rows of lb+1 ints taken from scratch (reallocated when
// too small), so repeated calls reuse the same buffer.
func damerauLevenshteinScratch(a, b string, scratch []int) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}

	if lb == 0 {
		return la
	}
	if len(scratch) < 3*(lb+1) {
		scratch = make([]int, 3*(lb+1))
	}
	w := lb + 1
	for j := range scratch[:w] {
		scratch[j] = j
	}

	for i := 1; i <= la; i++ {
		cur := (i % 3) * w
		prev := ((i + 2) % 3) * w
		prev2 := ((i + 1) % 3) * w
		scratch[cur] = i
		for j := 1; j <= lb; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}

			v := min(
				scratch[prev+j]+1,      // deletion
				scratch[cur+j-1]+1,     // insertion
				scratch[prev+j-1]+cost, // substitution
			)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				// transposition
				if d := scratch[prev2+j-2] + 1; d < v {
					v = d
				}
			}
			scratch[cur+j] = v
		}
	}

	return scratch[(la%3)*w+lb]
}
