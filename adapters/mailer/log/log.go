package log

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/shared/traceprop"
)

// checker renders messages as JSON lines without sending.
type checker struct {
	mu     sync.Mutex
	w      io.Writer
	buf    bytes.Buffer
	msg    logMessage
	closed atomic.Bool
}

var _ mailer.Mailer = (*checker)(nil)

// New validates opts and returns a Mailer writing single JSON lines to os.Stdout.
func New(opts mailer.Options) (mailer.Mailer, error) {
	return NewWithWriter(opts, os.Stdout)
}

// NewWithWriter validates opts and returns a Mailer writing single JSON lines to w.
// A nil w defaults to os.Stdout.
func NewWithWriter(opts mailer.Options, w io.Writer) (mailer.Mailer, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}
	if w == nil {
		w = os.Stdout
	}
	return &checker{w: w}, nil
}

type logAddress struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

type logAttachment struct {
	Name      string `json:"name"`
	Size      int    `json:"size"`
	Inline    bool   `json:"inline"`
	ContentID string `json:"content_id,omitempty"`
}

type logMessage struct {
	From        logAddress      `json:"from"`
	To          []logAddress    `json:"to"`
	Cc          []logAddress    `json:"cc"`
	Bcc         []logAddress    `json:"bcc"`
	Subject     string          `json:"subject"`
	Body        string          `json:"body"`
	HTML        string          `json:"html"`
	Attachments []logAttachment `json:"attachments"`
	// TraceID carries the W3C trace ID from ctx for log correlation.
	// Empty when ctx holds no valid span. SMTP has no header channel for
	// correlation without inventing protocol headers, so this is log-attrs
	// only; the MIME body is never mutated.
	TraceID string `json:"trace_id,omitempty"`
}

func toLogAddress(a mailer.Address) logAddress {
	return logAddress{Name: a.Name, Address: a.Address}
}

// Send validates msg and writes one JSON line. Attachment contents are
// redacted: only the size is logged, never raw bytes.
func (c *checker) Send(ctx context.Context, msg *mailer.Mail) error {
	if msg == nil {
		return mailer.ErrNilMessage
	}
	if c.closed.Load() {
		return mailer.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := msg.From.Validate(); err != nil {
		return fmt.Errorf("log: invalid from: %w", err)
	}
	if len(msg.To)+len(msg.Cc)+len(msg.Bcc) == 0 {
		return mailer.ErrNoRecipients
	}
	for i, a := range msg.To {
		if err := a.Validate(); err != nil {
			return fmt.Errorf("log: invalid to[%d]: %w", i, err)
		}
	}
	for i, a := range msg.Cc {
		if err := a.Validate(); err != nil {
			return fmt.Errorf("log: invalid cc[%d]: %w", i, err)
		}
	}
	for i, a := range msg.Bcc {
		if err := a.Validate(); err != nil {
			return fmt.Errorf("log: invalid bcc[%d]: %w", i, err)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() {
		return mailer.ErrClosed
	}
	m := &c.msg
	m.From = toLogAddress(msg.From)
	m.To = m.To[:0]
	for _, a := range msg.To {
		m.To = append(m.To, toLogAddress(a))
	}
	m.Cc = m.Cc[:0]
	for _, a := range msg.Cc {
		m.Cc = append(m.Cc, toLogAddress(a))
	}
	m.Bcc = m.Bcc[:0]
	for _, a := range msg.Bcc {
		m.Bcc = append(m.Bcc, toLogAddress(a))
	}
	m.Subject = msg.Subject
	m.Body = msg.Body
	m.HTML = msg.HTML
	m.Attachments = m.Attachments[:0]
	for _, a := range msg.Attachments {
		m.Attachments = append(m.Attachments, logAttachment{
			Name:      a.Name,
			Size:      len(a.Content),
			Inline:    a.Inline,
			ContentID: a.ContentID,
		})
	}
	m.TraceID = traceprop.TraceID(ctx)
	c.buf.Reset()
	// MarshalWrite emits no trailing newline; add the line terminator before
	// the single write so the wire format matches json.Marshal plus newline.
	if err := json.MarshalWrite(&c.buf, m, json.Deterministic(true)); err != nil {
		return err
	}
	c.buf.WriteByte('\n')
	_, err := c.w.Write(c.buf.Bytes())
	return err
}

// Close shuts down the checker. It is idempotent and always returns nil.
func (c *checker) Close() error {
	c.closed.Store(true)
	return nil
}
