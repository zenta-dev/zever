package diag

import "strings"

// Closest returns the candidate with the smallest edit distance to input,
// or "" when nothing is close enough (distance >2, or >3 for inputs longer
// than 6 runes; inputs of length <=3 require distance <=1). Case-insensitive,
// with a one-point bonus for prefix matches.
//
// This mirrors config/suggest.go's closest/damerauLevenshtein pair by design:
// diag is the compiler's zero-dependency foundation package and cannot import
// config (or cmd/zever), and the algorithm is small enough that duplicating it
// here -- so every unknown-name diagnostic can carry a "did you mean ...?"
// hint -- is cheaper than extracting a shared package for one function pair.
func Closest(input string, candidates []string) string {
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

// HintSuffix returns "" when no candidate is close to input, else
// ` (did you mean "candidate"?)`, ready to append to a diagnostic message.
func HintSuffix(input string, candidates []string) string {
	if s := Closest(input, candidates); s != "" {
		return " (did you mean \"" + s + "\"?)"
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
