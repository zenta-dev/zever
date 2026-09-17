package main

import "strings"

// all known top-level subcommands (including help aliases).
var allTopLevel = []string{
	"new", "compile", "check", "fmt", "doctor", "routes", "explain", "check-boundaries", "graph",
	"generate", "extract", "serve", "dev", "queue:work", "schedule:run", "tinker", "db", "help",
}

// generate subcommands.
var allGenerate = []string{
	"module", "entity", "job", "schedule", "server", "worker", "seed", "tinker", "adapter",
}

// db subcommands.
var allDB = []string{"migrate", "rollback", "seed"}

// backend names for compile.
var allBackends = []string{"proto", "zenorm", "atlas", "openapi", "gogen", "protogogen"}

// closest returns the candidate with smallest edit distance to input, if distance
// is within a threshold (≤2, or ≤3 for longer strings). Case-insensitive.
func closest(input string, candidates []string) string {
	if len(candidates) == 0 || input == "" {
		return ""
	}

	lower := strings.ToLower(input)
	best := ""
	bestDist := 999

	for _, c := range candidates {
		d := damerauLevenshtein(lower, strings.ToLower(c))
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
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}

	if lb == 0 {
		return la
	}
	// DP table.
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}

	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}

			d[i][j] = min(
				d[i-1][j]+1,      // deletion
				d[i][j-1]+1,      // insertion
				d[i-1][j-1]+cost, // substitution
			)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				// transposition
				if d[i-2][j-2]+1 < d[i][j] {
					d[i][j] = d[i-2][j-2] + 1
				}
			}
		}
	}

	return d[la][lb]
}
