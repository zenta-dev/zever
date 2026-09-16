package postgres

import (
	"fmt"
	"strconv"
	"strings"
)

// version is a parsed Postgres server version, split into its numeric
// components so capability checks can compare against feature thresholds
// (12.0 for the CTE MATERIALIZED modifier, 14.0 for recursive SEARCH/CYCLE,
// 15.0 for MERGE).
type version struct {
	major int
	minor int
	patch int
}

// atLeast reports whether v is >= (major, minor, patch).
func (v version) atLeast(major, minor, patch int) bool {
	if v.major != major {
		return v.major > major
	}

	if v.minor != minor {
		return v.minor > minor
	}

	return v.patch >= patch
}

// parseVersion parses a Postgres server version string such as "16.2",
// "15.0", or "16.3 (Ubuntu 16.3-1)" (any " (...)" or "-suffix" is ignored).
// Missing minor/patch components default to zero, so "15" parses as 15.0.0.
// More than three numeric components is an error, never a silent truncation.
// It returns an error for empty or non-numeric input rather than guessing, so
// capability decisions are never built on a silently-misparsed version.
func parseVersion(s string) (version, error) {
	if i := strings.IndexAny(s, " -("); i >= 0 {
		s = s[:i]
	}

	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 || parts[0] == "" {
		return version{}, fmt.Errorf("orm/dialect/postgres: invalid version %q", s)
	}

	nums := make([]int, 0, 3)

	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return version{}, fmt.Errorf("orm/dialect/postgres: invalid version %q: %w", s, err)
		}

		nums = append(nums, n)
	}

	for len(nums) < 3 {
		nums = append(nums, 0)
	}

	return version{major: nums[0], minor: nums[1], patch: nums[2]}, nil
}

// NewWithVersion returns a Postgres Dialect for a specific server version,
// e.g. "14.0" or "11.5". An unparsable version string is an error, never a
// silent fallback or panic: capability decisions must not be built on a
// guessed version. New targets a modern server (16.0); NewWithVersion
// exists for the version-gate tests and for callers pinned to an older
// server whose capabilities (CTE MATERIALIZED, SEARCH/CYCLE, MERGE) must be
// gated.
func NewWithVersion(s string) (Dialect, error) {
	v, err := parseVersion(s)
	if err != nil {
		return Dialect{}, err
	}

	return Dialect{version: v}, nil
}
