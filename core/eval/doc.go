// Package eval scores agent and generation outputs against expected answers.
//
// It owns datasets, scorers, and suite reports. Exact and substring scorers
// are deterministic and dependency-free; the judge scorer delegates to an
// ai.AI backend. It does not run agents itself; callers supply a RunFunc.
package eval
