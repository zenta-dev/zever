package log

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/notification"
)

// checker renders notifications as JSON lines without sending.
// It echoes the full notification including target: disable in production.
type checker struct {
	mu     sync.Mutex
	w      io.Writer
	codec  codec.Codec[logNotification]
	now    func() time.Time
	closed atomic.Bool
}

var _ notification.Notifier = (*checker)(nil)

// New validates opts and returns a Notifier writing single JSON lines to os.Stdout.
func New(opts notification.Options) (notification.Notifier, error) {
	return NewWithWriter(opts, os.Stdout)
}

// NewWithWriter validates opts and returns a Notifier writing single JSON lines to w.
// A nil w defaults to os.Stdout.
func NewWithWriter(opts notification.Options, w io.Writer) (notification.Notifier, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}
	if w == nil {
		w = os.Stdout
	}
	return &checker{w: w, codec: codec.JSONCodec[logNotification]{}, now: time.Now}, nil
}

type logNotification struct {
	TS         time.Time         `json:"ts"`
	Channel    string            `json:"channel"`
	Target     string            `json:"target"`
	Title      string            `json:"title"`
	Body       string            `json:"body"`
	Data       map[string]string `json:"data,omitempty"`
	Priority   string            `json:"priority"`
	TTLSeconds float64           `json:"ttl_seconds"`
}

// Notify validates n and writes one JSON line echoing the full notification
// including target. Disable this sink in production.
func (c *checker) Notify(ctx context.Context, n *notification.Notification) error {
	if n == nil {
		return notification.ErrNilNotification
	}
	if c.closed.Load() {
		return notification.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := n.Validate(); err != nil {
		return err
	}
	out := logNotification{
		TS:         c.now(),
		Channel:    string(n.Channel),
		Target:     n.Target,
		Title:      n.Title,
		Body:       n.Body,
		Data:       n.Data,
		Priority:   string(n.Priority),
		TTLSeconds: n.TTL.Seconds(),
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() {
		return notification.ErrClosed
	}
	data, err := c.codec.Encode(out)
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}
	if _, err := c.w.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("log: %w", err)
	}
	return nil
}

// Close shuts down the checker. It is idempotent and always returns nil.
func (c *checker) Close() error {
	c.closed.Swap(true)
	return nil
}
