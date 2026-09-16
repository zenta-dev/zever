package orm

import "sync/atomic"

// QueryLogger is the hook signature for capturing every query orm executes:
// the rendered SQL text (with the dialect's placeholders) plus the bound
// argument values, exactly as handed to the underlying driver. SetQueryLogger
// installs one; each execution site calls it immediately before issuing the
// query, so captures appear in execution order on the calling goroutine.
//
// Hooked surfaces: Query[T].All/First/Exists/Stream and Count
// (orm/stream.go, orm/query.go), plus the join, mutation and preload paths.
// Values are logged post dialect-encoding (e.g. sqlite timestamps as
// RFC3339 text), matching what the driver actually receives.
type QueryLogger func(query string, args []any)

// queryLoggerState boxes the active logger so the atomic slot can hold nil
// (unset) as its zero state.
type queryLoggerState struct {
	fn QueryLogger
}

// activeQueryLogger is the process-wide hook slot. It is deliberately an
// atomic pointer (not a mutex-guarded field) so the hot path -- logQuery on
// every executed query -- is a single lock-free load when no logger is set
// ("zero-cost when unset"), and so SetQueryLogger and concurrent query
// execution never race.
var activeQueryLogger atomic.Pointer[queryLoggerState]

// SetQueryLogger installs l as the process-wide query logger and returns a
// restore func that reinstates whatever logger (or nil) was active before.
// l may be nil, which disables logging. Set/restore calls may be nested:
// each restore pops back to its own preceding state. The hook is called on
// the goroutine that executes each query, in execution order.
func SetQueryLogger(l QueryLogger) (restore func()) {
	prev := activeQueryLogger.Load()

	if l == nil {
		activeQueryLogger.Store(nil)
	} else {
		activeQueryLogger.Store(&queryLoggerState{fn: l})
	}

	return func() {
		activeQueryLogger.Store(prev)
	}
}

// logQuery hands query+args to the active logger, if one is installed. It
// is the single hot-path hook every execution site calls; when unset it
// costs one atomic load and returns.
func logQuery(query string, args []any) {
	if st := activeQueryLogger.Load(); st != nil {
		st.fn(query, args)
	}
}
