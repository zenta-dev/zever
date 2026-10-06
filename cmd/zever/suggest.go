package main

import "strings"

// all known top-level subcommands (including help aliases).
var allTopLevel = []string{
	"new", "add", "compile", "check", "breaking", "fmt", "doctor", "config", "routes", "explain", "check-boundaries", "check:boundaries", "graph",
	"generate", "extract", "serve", "dev", "queue:work", "schedule:run", "tinker", "db", "help",
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
