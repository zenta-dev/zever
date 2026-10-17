package container

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"google.golang.org/grpc"
)

type dedupEntry struct {
	ptr uintptr
	typ reflect.Type
}

// DefaultCloseTimeout bounds per-service shutdown without a parent deadline.
const DefaultCloseTimeout = 5 * time.Second

// dedupKey reports the identity key for v, or false when v cannot dedupe.
// Only the service value's own reference identity dedupes: pointer, chan,
// map, func, and slice kinds key on reflect.Value.Pointer. Struct values
// never dedupe — two structs embedding the same pointer are distinct
// resources and each must close, so keying on an embedded pointer would
// wrongly skip the second service.
func dedupKey(v any) (dedupEntry, bool) {
	if v == nil {
		return dedupEntry{}, false
	}

	rv := reflect.ValueOf(v)

	//nolint:exhaustive // only reference kinds are deduplicable; default covers the rest
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
		if rv.IsNil() {
			return dedupEntry{}, false
		}

		return dedupEntry{ptr: rv.Pointer(), typ: rv.Type()}, true
	default:
		return dedupEntry{}, false
	}
}

type closeSnapshot struct {
	v  any
	ok bool
}

// snap resolves one lazy service into a closeSnapshot. It is generic so each
// lazy field's distinct instantiation can be listed in the snapshots table
// without a per-field closure.
func snap[T any](get func() (T, bool)) closeSnapshot {
	v, ok := get()
	return closeSnapshot{v, ok}
}

// snapshots returns close snapshots for every lazy service that has no
// explicit dependency ordering. Intentionally excluded (handled in Close's
// ordered section): cache, queue (dependencies, closed last), db (its pool
// lives in the DSN registry and closes with the registry band),
// scheduler, job, agent, rag (dependents that hold backend references,
// closed first),
// grpcServer (separate GracefulStop handling), grpcClients (separate
// per-conn close handling). This list must cover all remaining lazy
// fields; currently 33 entries + 9 ordered = 42 lazy fields. When
// adding a new service, add it here unless it depends on cache/queue
// (then add to Close's ordered section and keep excluded here). Drift is
// pinned by TestContainer_Snapshots_CoversAllServices via reflection.
func (c *Container) snapshots() []closeSnapshot {
	return []closeSnapshot{
		snap(c.ai.getIfResolved),
		snap(c.analytics.getIfResolved),
		snap(c.auth.getIfResolved),
		snap(c.billing.getIfResolved),
		snap(c.cdn.getIfResolved),
		snap(c.crypto.getIfResolved),
		snap(c.document.getIfResolved),
		snap(c.eventbus.getIfResolved),
		snap(c.flag.getIfResolved),
		snap(c.geo.getIfResolved),
		snap(c.i18n.getIfResolved),
		snap(c.idempotency.getIfResolved),
		snap(c.lock.getIfResolved),
		snap(c.log.getIfResolved),
		snap(c.mailer.getIfResolved),
		snap(c.media.getIfResolved),
		snap(c.notification.getIfResolved),
		snap(c.observability.getIfResolved),
		snap(c.outbox.getIfResolved),
		snap(c.password.getIfResolved),
		snap(c.payment.getIfResolved),
		snap(c.permission.getIfResolved),
		snap(c.ratelimit.getIfResolved),
		snap(c.resilience.getIfResolved),
		snap(c.router.getIfResolved),
		snap(c.search.getIfResolved),
		snap(c.secrets.getIfResolved),
		snap(c.session.getIfResolved),
		snap(c.storage.getIfResolved),
		snap(c.tenant.getIfResolved),
		snap(c.vectorstore.getIfResolved),
		snap(c.webhook.getIfResolved),
		snap(c.workflow.getIfResolved),
	}
}

// snapshotServiceNames parallels snapshots() order. It names the service for
// error reporting so close errors identify the service without ever
// including resolved values.
func snapshotServiceNames() []string {
	return []string{
		"ai", "analytics", "auth", "billing", "cdn", "crypto", "document",
		"eventbus", "flag", "geo", "i18n", "idempotency", "lock", "log", "mailer",
		"media", "notification", "observability", "outbox", "password", "payment",
		"permission", "ratelimit", "resilience", "router", "search", "secrets", "session", "storage",
		"tenant", "vectorstore", "webhook", "workflow",
	}
}

// Close closes every service this Container has already resolved. Services
// never touched are left alone — Close never opens anything. Not every
// service interface declares a Close method, so closeAny probes for whichever
// shape the resolved instance actually has: Close(ctx), Close(), Stop(), or
// Shutdown(ctx).
//
// Dependency-aware ordering derived from explicit dependency map:
//
//	scheduler -> queue (via job)
//	job       -> queue
//	*         -> (no dep, via snapshots)
//	cache, queue -> (leaf dependencies, closed last)
//	db, registry pools -> (shared pools, after every borrower so no
//	                       use-after-close; borrowers close as no-ops)
//	grpcServer -> (independent)
//	grpcClients -> (independent, one conn per target)
//
// Scheduler and job are closed first since they may hold a live reference
// to the shared queue instance injected by Job (which Scheduler resolves
// transitively); closing queue while scheduler could still be ticking would
// be a use-after-close. When adding a new service that depends on cache/queue,
// add it to the ordered section below (before snapshots or near scheduler/job)
// and keep it excluded from snapshots(); otherwise add it to snapshots().
func (c *Container) Close(ctx context.Context) error {
	var errs []error

	// seen is local to this Close call and only ever touched by tryClose,
	// which runs sequentially on the calling goroutine: no concurrent access
	// is possible, so a plain map suffices and avoids sync.Map's per-key
	// entry allocations. It is created lazily so an all-unresolved Close
	// allocates nothing.
	var seen map[dedupEntry]struct{}

	// keepAlive holds every deduped value for the duration of Close so the
	// garbage collector cannot reclaim an address while its pointer is used
	// as a dedup key and then reuse that address for a later allocation,
	// which would cause a false dedup hit.
	keepAlive := make([]any, 0, 32)

	// Container lifetime is over: cancel the builder-facing root context so
	// any in-flight or future build winds down instead of running against a
	// closing container.
	c.cancelRoot()

	// done carries one service's shutdown result. It is reused across
	// services: a completed close drains it and its goroutine exits, so the
	// next close can safely reuse the same buffered channel. A timed-out
	// close abandons its goroutine still holding the channel, so a fresh one
	// is allocated before the next close. Created lazily so an
	// all-unresolved Close allocates nothing.
	var done chan error

	tryClose := func(service string, v any) {
		if v == nil {
			return
		}

		if key, ok := dedupKey(v); ok {
			if seen == nil {
				seen = make(map[dedupEntry]struct{}, 8)
			}

			if _, loaded := seen[key]; loaded {
				return
			}

			seen[key] = struct{}{}
			keepAlive = append(keepAlive, v)
		}

		// Per-service bounded shutdown: derive timeout from parent ctx so
		// Close never blocks forever even if a service Close ignores ctx.
		// A 5s timeout applies only when the parent carries no deadline;
		// otherwise the parent deadline bounds the wait.
		perCtx := ctx

		var cancel context.CancelFunc
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			perCtx, cancel = context.WithTimeout(ctx, DefaultCloseTimeout)
		}

		if done == nil {
			done = make(chan error, 1)
		}

		// Pass the channel by value so an abandoned goroutine from a timed-out
		// close keeps its own channel even when done is reassigned below.
		ch := done

		go closeAsync(perCtx, v, service, ch)

		select {
		case err := <-ch:
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
					errs = append(errs, fmt.Errorf("container: close %s: %w", service, err))
				}
			}
		case <-perCtx.Done():
			if cancel != nil {
				cancel()
			}

			errs = append(errs, CloseTimeoutError{Service: service, Abandoned: true, Err: perCtx.Err()})

			c.logAbandonedCloser(service, perCtx.Err())

			// Don't block on the abandoned goroutine; it will finish and be
			// GC'd. Give the next close a fresh channel so a late send cannot
			// be mistaken for that close's result.
			done = make(chan error, 1)
		}
	}

	if v, ok := c.scheduler.getIfResolved(); ok {
		tryClose("scheduler", v)
	}

	if v, ok := c.job.getIfResolved(); ok {
		tryClose("job", v)
	}

	if v, ok := c.agent.getIfResolved(); ok {
		tryClose("agent", v)
	}

	if v, ok := c.rag.getIfResolved(); ok {
		tryClose("rag", v)
	}

	// Explicitly wired providers close before the legacy bands, in
	// reverse dependency order (dependents first), labeled service[/name].
	// Pointer dedup is shared with the bands below: a provided instance
	// that is also the legacy singleton closes once.
	if len(c.provDefs) > 0 {
		snaps := c.providerSnapshots()
		names := c.providerSnapshotNames()
		for i := range snaps {
			if snaps[i].ok {
				tryClose(names[i], snaps[i].v)
			}
		}
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

	// Shared-pool band: the db service (its pool may live in the registry
	// or stand alone for :memory:) and every registry pool close after all
	// borrowers. Borrowers hold owns=false wrappers whose Close is a no-op,
	// so the pool is reached exactly once here; pointer dedup additionally
	// guards the case where c.db is the registry pool itself.
	if v, ok := c.db.getIfResolved(); ok {
		tryClose("db", v)
	}

	if c.pools != nil {
		for _, ps := range c.pools.snapshot() {
			tryClose(ps.name, ps.conn)
		}
	}

	for _, ps := range c.pluginSnapshots() {
		if ps.ok {
			tryClose("plugin "+ps.name, ps.v)
		}
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

	// gRPC client conns are independent of cache/queue/scheduler/job, so
	// they close here rather than in the dependency-ordered sequence. The
	// map is snapshotted under the cache lock so a concurrent GRPCClient
	// build cannot race the iteration; each conn closes once via tryClose,
	// whose pointer dedup also guards the same conn reached twice.
	if cache, ok := c.grpcClients.getIfResolved(); ok {
		cache.mu.Lock()
		conns := make([]*grpc.ClientConn, 0, len(cache.conns))
		for _, conn := range cache.conns {
			conns = append(conns, conn)
		}
		cache.mu.Unlock()

		for _, conn := range conns {
			tryClose("grpc client", conn)
		}
	}

	_ = keepAlive

	return errors.Join(errs...)
}

// logAbandonedCloser reports a timed-out close whose goroutine was
// abandoned via the container logger when one is resolved. The warn is
// best-effort: an unresolved logger skips it, and logging must never block
// or fail Close.
func (c *Container) logAbandonedCloser(service string, cause error) {
	lg, ok := c.log.getIfResolved()
	if !ok {
		return
	}

	lg.Warn().
		Str("service", service).
		AnErr("error", cause).
		Msgf("container: close %s timed out, abandoning goroutine", service)
}

// closeAsync runs closeAny for one service on its own goroutine and funnels
// the result, or a recovered panic, to done. It is a top-level function so
// the go statement in Close allocates no closure; ctx, v, service, and done
// are passed by value.
func closeAsync(ctx context.Context, v any, service string, done chan<- error) {
	defer func() {
		if r := recover(); r != nil {
			done <- ClosePanicError{Service: service, Panic: r}
		}
	}()

	done <- closeAny(ctx, v)
}

// closeAny probes the resolved instance for a shutdown shape, in order:
// Close(context.Context) error, then Close() error, then Stop() error,
// then Shutdown(context.Context) error, then nil. The Stop() error probe
// exists for the scheduler service, whose interface declares Stop() error
// instead of Close. The Shutdown(context.Context) error probe exists for
// the observability service, whose Provider (and Tracer/Metrics) interfaces
// declare Shutdown instead of Close, so buffered spans/metrics are flushed
// on Container.Close. Anything else (for example *job.Dispatcher, which has
// no shutdown method) is a no-op nil.
func closeAny(ctx context.Context, v any) error {
	switch closer := v.(type) {
	case interface{ Close(context.Context) error }:
		return closer.Close(ctx)
	case interface{ Close() error }:
		return closer.Close()
	case interface{ Stop() error }:
		return closer.Stop()
	case interface{ Shutdown(context.Context) error }:
		return closer.Shutdown(ctx)
	default:
		return nil
	}
}
