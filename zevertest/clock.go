package zevertest

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultStart is the time a Clock created with a zero start reports.
var DefaultStart = time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)

// Clock is a manually advanced fake clock. It is safe for concurrent use.
// Pass Clock.Now (or Clock.Func) wherever code accepts a `func() time.Time`.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

// NewClock returns a Clock reading start, or DefaultStart when start is zero.
func NewClock(start time.Time) *Clock {
	if start.IsZero() {
		start = DefaultStart
	}

	return &Clock{now: start}
}

// Now returns the current fake time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

// Func returns Now as a `func() time.Time` value.
func (c *Clock) Func() func() time.Time { return c.Now }

// Set moves the clock to t.
func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = t
}

// Advance moves the clock forward by d (backward when negative) and returns
// the new time.
func (c *Clock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)

	return c.now
}

// IDs is a deterministic ID source: Next returns "<prefix>-000001",
// "<prefix>-000002", and so on. It is safe for concurrent use.
type IDs struct {
	prefix string
	n      atomic.Uint64
}

// NewIDs returns an ID source with the given prefix ("id" when empty).
func NewIDs(prefix string) *IDs {
	if prefix == "" {
		prefix = "id"
	}

	return &IDs{prefix: prefix}
}

// Next returns the next ID.
func (g *IDs) Next() string {
	return fmt.Sprintf("%s-%06d", g.prefix, g.n.Add(1))
}

// Func returns Next as a `func() string` value.
func (g *IDs) Func() func() string { return g.Next }
