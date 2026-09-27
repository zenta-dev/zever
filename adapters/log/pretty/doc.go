// Package pretty provides a human-readable log.Logger for local
// development: one timestamped, level-labeled line per event with sorted
// key=value fields.
//
// Color is emitted only when the destination is a terminal and neither
// NO_COLOR nor TERM=dumb disables it, so piped or redirected output never
// leaks ANSI escapes.
package pretty
