package container

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
	"unsafe"
)

type dedupEntry struct {
	ptr unsafe.Pointer
	typ reflect.Type
}

func dedupKey(v any) (dedupEntry, bool) {
	if v == nil {
		return dedupEntry{}, false
	}

	rv := reflect.ValueOf(v)

	//nolint:exhaustive // only reference kinds are deduplicable; default covers the rest
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		if rv.IsNil() {
			return dedupEntry{}, false
		}

		//nolint:gosec // pointer identity for dedup, keepAlive prevents reuse
		return dedupEntry{ptr: unsafe.Pointer(rv.Pointer()), typ: rv.Type()}, true
	case reflect.Struct:
		// Struct value services are not reference types: two copies of the same
		// struct with identical pointer fields share the underlying resource but
		// have distinct interface values. Best-effort dedup: if struct wraps a
		// non-nil reference field (pointer, interface holding pointer, slice/map/chan
		// wrapping shared state, or nested struct containing one), dedup on that
		// pointer to avoid double-close of the shared resource. Otherwise not
		// deduplicable; caller must ensure Close is idempotent.
		if ptr, typ, ok := findPointerInValue(rv); ok {
			//nolint:gosec // pointer identity for struct-wrapped resource
			return dedupEntry{ptr: ptr, typ: typ}, true
		}

		return dedupEntry{}, false
	default:
		return dedupEntry{}, false
	}
}

// findPointerInValue walks a struct value (and nested structs/interfaces) to find the
// first non-nil reference pointer for dedup. It handles pointer, interface (holding
// pointer/struct), and nested struct cases so that struct-value services that wrap
// a shared resource via an interface or nested struct are still deduped.
func findPointerInValue(rv reflect.Value) (unsafe.Pointer, reflect.Type, bool) {
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		switch f.Kind() { //nolint:exhaustive // remaining kinds are non-deduplicable scalars
		case reflect.Pointer, reflect.Chan, reflect.Map, reflect.Func, reflect.Slice, reflect.UnsafePointer:
			if f.IsNil() {
				continue
			}
			//nolint:gosec // pointer identity for dedup
			return unsafe.Pointer(f.Pointer()), f.Type(), true
		case reflect.Interface:
			if f.IsNil() {
				continue
			}

			elem := f.Elem()
			switch elem.Kind() { //nolint:exhaustive // remaining kinds are non-deduplicable scalars
			case reflect.Pointer, reflect.Chan, reflect.Map, reflect.Func, reflect.Slice, reflect.UnsafePointer:
				if elem.IsNil() {
					continue
				}
				//nolint:gosec // pointer identity for interface-wrapped resource
				return unsafe.Pointer(elem.Pointer()), elem.Type(), true
			case reflect.Struct:
				if ptr, typ, ok := findPointerInValue(elem); ok {
					return ptr, typ, true
				}
			default:
				// non-pointer interface payloads cannot be deduped
			}
		case reflect.Struct:
			if ptr, typ, ok := findPointerInValue(f); ok {
				return ptr, typ, true
			}
		default:
			// non-pointer kinds cannot be deduped
		}
	}

	return nil, nil, false
}

type closeSnapshot struct {
	v  any
	ok bool
}

// snapshots returns close snapshots for every lazy service that has no
// explicit dependency ordering. Intentionally excluded (handled in Close's
// ordered section): cache, queue (dependencies, closed last), scheduler, job
// (dependents that hold cache/queue references, closed first), grpcServer
// (separate GracefulStop handling). This list must cover all remaining lazy
// fields; currently 31 entries + 5 ordered = 36 lazy fields. When adding a
// new service, add it here unless it depends on cache/queue (then add to
// Close's ordered section and keep excluded here). Drift is pinned by
// TestContainer_Snapshots_CoversAllServices via reflection.
func (c *Container) snapshots() []closeSnapshot {
	return []closeSnapshot{
		func() closeSnapshot { v, ok := c.ai.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.analytics.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.auth.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.billing.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.crypto.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.db.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.document.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.eventbus.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.flag.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.geo.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.i18n.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.idempotency.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.lock.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.log.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.mailer.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.media.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.notification.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.observability.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.password.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.payment.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.permission.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.ratelimit.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.router.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.search.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.secrets.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.session.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.storage.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.tenant.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.vectorstore.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.webhook.getIfResolved(); return closeSnapshot{v, ok} }(),
		func() closeSnapshot { v, ok := c.workflow.getIfResolved(); return closeSnapshot{v, ok} }(),
	}
}

// snapshotServiceNames parallels snapshots() order. It names the service for
// error reporting so close errors identify the service without ever
// including resolved values.
func snapshotServiceNames() []string {
	return []string{
		"ai", "analytics", "auth", "billing", "crypto", "db", "document",
		"eventbus", "flag", "geo", "i18n", "idempotency", "lock", "log", "mailer",
		"media", "notification", "observability", "password", "payment",
		"permission", "ratelimit", "router", "search", "secrets", "session", "storage",
		"tenant", "vectorstore", "webhook", "workflow",
	}
}

// Close closes every service this Container has already resolved. Services
// never touched are left alone — Close never opens anything. Not every
// service interface declares a Close method, so closeAny probes for whichever
// shape the resolved instance actually has.
//
// Dependency-aware ordering derived from explicit dependency map:
//
//	scheduler -> cache, queue
//	job       -> queue
//	*         -> (no dep, via snapshots)
//	cache, queue -> (leaf dependencies, closed last)
//	grpcServer -> (independent)
//
// Scheduler and job are closed first since they may hold a live reference
// to the shared cache/queue instances injected by Scheduler/Job above;
// closing cache/queue while scheduler could still be ticking would be a
// use-after-close. When adding a new service that depends on cache/queue,
// add it to the ordered section below (before snapshots or near scheduler/job)
// and keep it excluded from snapshots(); otherwise add it to snapshots().
func (c *Container) Close(ctx context.Context) error {
	var errs []error

	var seen sync.Map

	// keepAlive holds every deduped value for the duration of Close so the
	// garbage collector cannot reclaim an address while its pointer is used
	// as a dedup key and then reuse that address for a later allocation,
	// which would cause a false dedup hit.
	keepAlive := make([]any, 0, 32)

	tryClose := func(service string, v any) {
		if v == nil {
			return
		}

		if key, ok := dedupKey(v); ok {
			if _, loaded := seen.LoadOrStore(key, struct{}{}); loaded {
				return
			}

			keepAlive = append(keepAlive, v)
		}

		// Per-service bounded shutdown: derive timeout from parent ctx so
		// Close never blocks forever even if a service Close ignores ctx.
		// A 5s timeout applies only when the parent carries no deadline;
		// otherwise the parent deadline bounds the wait.
		perCtx := ctx

		var cancel context.CancelFunc
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			perCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
		}

		done := make(chan error, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					done <- ClosePanicError{Service: service, Panic: r}
				}
			}()

			done <- closeAny(perCtx, v)
		}()

		select {
		case err := <-done:
			if cancel != nil {
				cancel()
			}

			if err != nil {
				var panicErr ClosePanicError
				var timeoutErr CloseTimeoutError
				if errors.As(err, &panicErr) || errors.As(err, &timeoutErr) {
					errs = append(errs, err)
				} else {
					// Name the service only, never resolved values.
					errs = append(errs, fmt.Errorf("[container] close %s: %w", service, err))
				}
			}
		case <-perCtx.Done():
			if cancel != nil {
				cancel()
			}

			errs = append(errs, CloseTimeoutError{Service: service, Err: perCtx.Err()})

			// Don't block on done goroutine; it will finish and be GC'd.
		}
	}

	if v, ok := c.scheduler.getIfResolved(); ok {
		tryClose("scheduler", v)
	}

	if v, ok := c.job.getIfResolved(); ok {
		tryClose("job", v)
	}

	snaps := c.snapshots()
	names := snapshotServiceNames()
	for i := range snaps {
		if snaps[i].ok {
			tryClose(names[i], snaps[i].v)
		}
	}

	if v, ok := c.cache.getIfResolved(); ok {
		tryClose("cache", v)
	}

	if v, ok := c.queue.getIfResolved(); ok {
		tryClose("queue", v)
	}

	// gRPC has no dependency on (and nothing depends on) cache/queue/
	// scheduler/job, so it is stopped here rather than woven into the
	// dependency-ordered sequence above -- only its own resolved-check
	// gates whether anything happens. GracefulStop() blocks until every
	// in-flight RPC finishes, so it is run in its own goroutine bounded by
	// ctx, matching how the rest of Close never lets one slow shutdown
	// starve the others.
	if srv, ok := c.grpcServer.getIfResolved(); ok {
		stopped := make(chan struct{})

		go func() {
			srv.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
		case <-ctx.Done():
			srv.Stop()
			<-stopped
		}
	}

	_ = keepAlive

	return errors.Join(errs...)
}

// closeAny probes the resolved instance for a shutdown shape, in order:
// Close(context.Context) error, then Close() error, then Stop() error,
// then nil. The Stop() error probe exists for the scheduler service, whose
// interface declares Stop() error instead of Close. Anything else (for
// example *job.Dispatcher, which has no shutdown method) is a no-op nil.
func closeAny(ctx context.Context, v any) error {
	switch closer := v.(type) {
	case interface{ Close(context.Context) error }:
		return closer.Close(ctx)
	case interface{ Close() error }:
		return closer.Close()
	case interface{ Stop() error }:
		return closer.Stop()
	default:
		return nil
	}
}
