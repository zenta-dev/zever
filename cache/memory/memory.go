package memory

import (
	"container/list"
	"context"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/zenta-dev/zever/cache"
)

const defaultMaxEntries = 1000

type item struct {
	value     []byte
	expiresAt time.Time
}

type memoryAdapter struct {
	mu         sync.RWMutex
	items      map[string]item
	l          *list.List
	index      map[string]*list.Element
	maxEntries int
	stop       chan struct{}
	wg         sync.WaitGroup
	once       sync.Once
	closed     bool
}

// New creates in-memory cache adapter using sweep interval default 1m and max entries default 1000, starts janitor.
func New(opts cache.Options) (cache.Cache, error) {
	interval := opts.SweepInterval
	if interval <= 0 {
		interval = time.Minute
	}

	maxEntries := opts.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultMaxEntries
	}

	a := &memoryAdapter{
		items:      make(map[string]item),
		l:          list.New(),
		index:      make(map[string]*list.Element),
		maxEntries: maxEntries,
		stop:       make(chan struct{}),
	}

	a.startJanitor(interval)

	return a, nil
}

func (a *memoryAdapter) Get(_ context.Context, key string) ([]byte, error) {
	now := time.Now()

	a.mu.RLock()

	if a.closed {
		a.mu.RUnlock()

		return nil, cache.ErrClosed
	}

	it, ok := a.items[key]
	a.mu.RUnlock()

	if !ok {
		return nil, &cache.NotFoundError{Key: key}
	}

	if a.isExpired(it, now) {
		return a.getExpired(key, now)
	}

	a.mu.Lock()
	a.promote(key)
	a.mu.Unlock()

	return append([]byte(nil), it.value...), nil
}

func (a *memoryAdapter) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return cache.ErrClosed
	}

	a.ensureOrder()

	it := item{value: append([]byte(nil), value...)}

	if ttl > 0 {
		it.expiresAt = time.Now().Add(ttl)
	}

	if _, exists := a.items[key]; !exists {
		a.index[key] = a.l.PushBack(key)
	}

	a.items[key] = it
	a.evictIfOverCap()

	return nil
}

func (a *memoryAdapter) SetIfAbsent(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return false, cache.ErrClosed
	}

	a.ensureOrder()

	now := time.Now()

	if it, ok := a.items[key]; ok && (it.expiresAt.IsZero() || now.Before(it.expiresAt)) {
		return false, nil
	}

	if _, ok := a.items[key]; ok {
		delete(a.items, key)
		a.removeFromOrder(key)
	}

	it := item{value: append([]byte(nil), value...)}

	if ttl > 0 {
		it.expiresAt = time.Now().Add(ttl)
	}

	a.items[key] = it
	a.index[key] = a.l.PushBack(key)
	a.evictIfOverCap()

	return true, nil
}

func (a *memoryAdapter) Delete(_ context.Context, key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return cache.ErrClosed
	}

	if _, ok := a.items[key]; ok {
		delete(a.items, key)
		a.removeFromOrder(key)
	}

	return nil
}

func (a *memoryAdapter) Increment(_ context.Context, key string) error {
	return a.addDelta(key, 1)
}

func (a *memoryAdapter) Decrement(_ context.Context, key string) error {
	return a.addDelta(key, -1)
}

func (a *memoryAdapter) addDelta(key string, delta int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return cache.ErrClosed
	}

	a.ensureOrder()

	now := time.Now()

	var (
		cur      int64
		expireAt time.Time
	)

	if it, ok := a.items[key]; ok {
		if a.isExpired(it, now) {
			delete(a.items, key)
			a.removeFromOrder(key)
		} else {
			n, err := strconv.ParseInt(string(it.value), 10, 64)
			if err != nil {
				return &cache.InvalidValueError{Key: key, Err: err}
			}

			cur = n
			expireAt = it.expiresAt
		}
	}

	newVal := cur + delta

	a.items[key] = item{
		value:     strconv.AppendInt(nil, newVal, 10),
		expiresAt: expireAt,
	}

	if _, ok := a.index[key]; !ok {
		a.index[key] = a.l.PushBack(key)
	} else {
		a.promote(key)
	}

	a.evictIfOverCap()

	return nil
}

func (a *memoryAdapter) Exists(_ context.Context, key string) (bool, error) {
	now := time.Now()

	a.mu.RLock()

	if a.closed {
		a.mu.RUnlock()

		return false, cache.ErrClosed
	}

	it, ok := a.items[key]
	a.mu.RUnlock()

	if !ok {
		return false, nil
	}

	if a.isExpired(it, now) {
		return a.existsExpired(key, now)
	}

	a.mu.Lock()
	a.promote(key)
	a.mu.Unlock()

	return true, nil
}

func (a *memoryAdapter) sweep() {
	now := time.Now()

	a.mu.RLock()

	var expired []string

	for k, it := range a.items {
		if a.isExpired(it, now) {
			expired = append(expired, k)
		}
	}

	a.mu.RUnlock()

	if len(expired) == 0 {
		return
	}

	for _, k := range expired {
		a.mu.Lock()

		cur, ok := a.items[k]
		if !ok {
			a.mu.Unlock()

			continue
		}

		if !a.isExpired(cur, now) {
			a.mu.Unlock()

			continue
		}

		delete(a.items, k)

		if el, ok := a.index[k]; ok {
			if a.l != nil {
				a.l.Remove(el)
			}

			delete(a.index, k)
		}

		a.mu.Unlock()
	}
}

func (a *memoryAdapter) Close(_ context.Context) error {
	a.once.Do(func() {
		a.mu.Lock()
		if a.closed {
			a.mu.Unlock()

			return
		}

		a.closed = true
		a.mu.Unlock()

		close(a.stop)
		a.wg.Wait()

		a.mu.Lock()
		a.items = make(map[string]item)
		if a.l != nil {
			a.l.Init()
		}

		if a.index != nil {
			clear(a.index)
		}

		a.mu.Unlock()
	})

	return nil
}

func (a *memoryAdapter) startJanitor(interval time.Duration) {
	a.wg.Add(1)

	a.wg.Go(func() {
		defer a.wg.Done()

		jitter := time.Duration(rand.Int64N(int64(interval/10) + 1)) //nolint:gosec // weak rand is fine for jitter
		if jitter > 0 {
			interval += jitter
		}

		t := time.NewTicker(interval)
		defer t.Stop()

		for {
			select {
			case <-t.C:
				a.sweep()
			case <-a.stop:
				return
			}
		}
	})
}

func (a *memoryAdapter) ensureOrder() {
	if a.l == nil {
		a.l = list.New()
	}

	if a.index == nil {
		a.index = make(map[string]*list.Element)
	}
}

func (a *memoryAdapter) isExpired(it item, now time.Time) bool {
	return !it.expiresAt.IsZero() && now.After(it.expiresAt)
}

func (a *memoryAdapter) promote(key string) {
	a.ensureOrder()

	if el, ok := a.index[key]; ok {
		a.l.MoveToBack(el)
	}
}

func (a *memoryAdapter) removeFromOrder(key string) {
	if a.index == nil {
		return
	}

	if el, ok := a.index[key]; ok {
		if a.l != nil {
			a.l.Remove(el)
		}

		delete(a.index, key)
	}
}

func (a *memoryAdapter) getExpired(key string, now time.Time) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil, cache.ErrClosed
	}

	cur, ok := a.items[key]
	if !ok {
		return nil, &cache.NotFoundError{Key: key}
	}

	if !a.isExpired(cur, now) {
		a.promote(key)

		return append([]byte(nil), cur.value...), nil
	}

	delete(a.items, key)
	a.removeFromOrder(key)

	return nil, &cache.NotFoundError{Key: key}
}

func (a *memoryAdapter) evictIfOverCap() {
	if a.maxEntries <= 0 {
		return
	}

	for len(a.items) > a.maxEntries {
		if a.l == nil || a.l.Len() == 0 {
			break
		}

		front := a.l.Front()
		if front == nil {
			break
		}

		k, _ := front.Value.(string)

		a.l.Remove(front)
		delete(a.items, k)
		delete(a.index, k)
	}
}

func (a *memoryAdapter) existsExpired(key string, now time.Time) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return false, cache.ErrClosed
	}

	cur, ok := a.items[key]
	if !ok {
		return false, nil
	}

	if !a.isExpired(cur, now) {
		a.promote(key)

		return true, nil
	}

	delete(a.items, key)
	a.removeFromOrder(key)

	return false, nil
}
