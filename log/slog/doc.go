// Package slog provides a log.Logger implementation backed by the standard
// library's log/slog package.
//
// The adapter writes JSON events to stdout. The log/slog package has no fatal
// level and never terminates the process, so log.LevelFatal maps to a custom
// slog level above error and renders as "ERROR+4" in JSON output.
package slog
