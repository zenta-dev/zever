package redisclient

import (
	"fmt"
	"strconv"
	"sync"

	goredis "github.com/redis/go-redis/v9"
)

// sharedEntry is one refcounted client in the registry. building marks the
// in-flight first open; concurrent borrowers wait on ready until it closes.
type sharedEntry struct {
	client   *goredis.Client
	refs     int
	building bool
	ready    chan struct{}
}

// sharedRegistry maps canonical connection keys to one refcounted client
// per key. It is safe for concurrent use: the first opener builds while
// concurrent borrowers wait on the entry channel; a build failure evicts
// the key so the next access retries. open is a seam so tests can force a
// failed build.
type sharedRegistry struct {
	mu    sync.Mutex
	items map[string]*sharedEntry
	open  func(*goredis.Options) (*goredis.Client, error)
}

func newSharedRegistry() *sharedRegistry {
	return &sharedRegistry{
		items: make(map[string]*sharedEntry),
		open: func(o *goredis.Options) (*goredis.Client, error) {
			return goredis.NewClient(o), nil
		},
	}
}

// shared is the process-wide registry backing Shared. Batteries targeting
// the same resolved addr+DB share one client/pool.
var shared = newSharedRegistry()

// Shared returns a refcounted client for opts, opening it on first use and
// reusing it while other borrowers hold a reference. The key covers the
// resolved host/port, DB, and TLS/password identity that affects the
// connection; pool-sizing fields are deliberately excluded, so the first
// caller's pool options win for every later borrower. release decrements
// the count; at zero it closes the client and removes the entry. release is
// idempotent and nil-safe. Callers that need a private client use New.
func Shared(opts Options) (*goredis.Client, func() error, error) {
	redisOpt, err := toRedisOptions(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("redis: new client: %w", err)
	}

	client, release, err := shared.acquire(sharedKey(redisOpt), redisOpt)
	if err != nil {
		return nil, nil, fmt.Errorf("redis: new client: %w", err)
	}

	return client, release, nil
}

// sharedKey derives the exact-string registry key from the resolved
// go-redis options. Matching is exact after toRedisOptions normalized the
// address: the same host/port/DB/password/TLS identity shares, and nothing
// else does. Pool sizing is not part of the key. The key is never logged.
func sharedKey(o *goredis.Options) string {
	tls := o.TLSConfig != nil

	return "redis:" + o.Addr +
		"|db=" + strconv.Itoa(o.DB) +
		"|tls=" + strconv.FormatBool(tls) +
		"|pw=" + o.Password
}

// acquire returns the shared client for key, building it on first use with
// opts. The returned release is idempotent and nil-safe.
func (r *sharedRegistry) acquire(key string, opts *goredis.Options) (*goredis.Client, func() error, error) {
	for {
		r.mu.Lock()

		e, ok := r.items[key]
		if !ok {
			e = &sharedEntry{building: true, ready: make(chan struct{})}
			r.items[key] = e
			r.mu.Unlock()

			client, err := r.open(opts)

			r.mu.Lock()
			if err != nil {
				delete(r.items, key)
				e.building = false
				r.mu.Unlock()
				close(e.ready)

				return nil, nil, err
			}

			e.client = client
			e.refs = 1
			e.building = false
			r.mu.Unlock()
			close(e.ready)

			return client, r.releaser(key, e), nil
		}

		if e.building {
			ready := e.ready
			r.mu.Unlock()
			<-ready

			continue
		}

		e.refs++
		client := e.client
		r.mu.Unlock()

		return client, r.releaser(key, e), nil
	}
}

// releaser returns a once-guarded release closure bound to one entry. A
// release for an entry already evicted or replaced is a no-op, so a stale
// borrower never closes a successor's client.
func (r *sharedRegistry) releaser(key string, e *sharedEntry) func() error {
	var (
		once sync.Once
		err  error
	)

	return func() error {
		once.Do(func() { err = r.release(key, e) })

		return err
	}
}

// release decrements the entry's refcount and, at zero, removes and closes
// it. It is nil-safe: a nil client closes nothing.
func (r *sharedRegistry) release(key string, e *sharedEntry) error {
	r.mu.Lock()

	cur, ok := r.items[key]
	if !ok || cur != e {
		r.mu.Unlock()

		return nil
	}

	e.refs--
	if e.refs > 0 {
		r.mu.Unlock()

		return nil
	}

	delete(r.items, key)
	client := e.client
	r.mu.Unlock()

	if client == nil {
		return nil
	}

	return Close(client)
}
