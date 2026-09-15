// Package firebase provides a Firebase Remote Config-backed flag.Flag.
//
// Evaluation is server-side: the template is loaded once at New (fail-fast;
// network or 5xx errors abort Open) and cached. There is no background
// refresh, so values go stale until the process restarts and loads again.
//
// Targeting uses flag.WithEvalContext: EvalContext.RandomizationID feeds
// percentage-rollout conditions and EvalContext.Signals are exposed as
// top-level custom-signal keys. An empty EvalContext deterministically
// misses percentage and signal conditions, yielding template defaults.
//
// Credentials come from a service-account JSON key file whose path is
// validated (clean, no traversal, .json extension, not a directory).
package firebase
