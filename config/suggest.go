package config

import "strings"

// closest returns the candidate with smallest edit distance to input, if the
// distance is within a threshold (≤2, or ≤3 for longer strings).
// Case-insensitive.
//
// This is a small, deliberately duplicated equivalent of
// cmd/zever/suggest.go's closest/damerauLevenshtein pair: config cannot
// import cmd/zever (that would invert the module's dependency direction),
// and the algorithm is small enough that duplicating it here -- to give
// UnknownServiceError/UnknownFieldError the same "did you mean %q?" hint
// every CLI-facing error path already has -- is cheaper than extracting a
// shared package for one function pair.
func closest(input string, candidates []string) string {
	if len(candidates) == 0 || input == "" {
		return ""
	}

	lower := strings.ToLower(input)
	best := ""
	bestDist := 999

	for _, c := range candidates {
		d := damerauLevenshtein(lower, strings.ToLower(c))
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

	if len(input) <= 3 && bestDist > 1 {
		return ""
	}

	if bestDist <= threshold {
		return best
	}

	return ""
}

func damerauLevenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}

	if lb == 0 {
		return la
	}

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
				d[i-1][j]+1,
				d[i][j-1]+1,
				d[i-1][j-1]+cost,
			)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				if d[i-2][j-2]+1 < d[i][j] {
					d[i][j] = d[i-2][j-2] + 1
				}
			}
		}
	}

	return d[la][lb]
}
