package sqlite

import (
	"fmt"
	"strconv"
	"strings"
)

// version is a parsed SQLite library version, split into its numeric
// components so capability checks can compare against feature thresholds.
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

// parseVersion parses a SQLite version string such as "3.46.0", "3.30.0",
// or "3.46.0-community" (any "-suffix" is ignored). Missing minor/patch
// components default to zero, so "3.30" parses as 3.30.0. More than three
// numeric components is an error, never a silent truncation. It returns an
// error for empty or non-numeric input rather than guessing, so capability
// decisions are never built on a silently-misparsed version.
func parseVersion(s string) (version, error) {
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s = s[:i]
	}

	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 || parts[0] == "" {
		return version{}, fmt.Errorf("orm/dialect/sqlite: invalid version %q", s)
	}

	nums := make([]int, 0, 3)

	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return version{}, fmt.Errorf("orm/dialect/sqlite: invalid version %q: %w", s, err)
		}

		nums = append(nums, n)
	}

	for len(nums) < 3 {
		nums = append(nums, 0)
	}

	return version{major: nums[0], minor: nums[1], patch: nums[2]}, nil
}
