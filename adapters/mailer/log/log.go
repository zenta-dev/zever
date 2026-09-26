package log

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/shared/codec"
)

// checker renders messages as JSON lines without sending.
type checker struct {
	mu     sync.Mutex
	w      io.Writer
	codec  codec.Codec[logMessage]
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
		return nil, fmt.Errorf("mailer: log: %w", err)
	}
	if w == nil {
		w = os.Stdout
	}
	return &checker{w: w, codec: codec.JSONCodec[logMessage]{}}, nil
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
}

func toLogAddress(a mailer.Address) logAddress {
	return logAddress{Name: a.Name, Address: a.Address}
}

func toLogAddresses(in []mailer.Address) []logAddress {
	out := make([]logAddress, 0, len(in))
	for _, a := range in {
		out = append(out, toLogAddress(a))
	}
	return out
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
		return fmt.Errorf("mailer: log invalid from: %w", err)
	}
	if len(msg.To)+len(msg.Cc)+len(msg.Bcc) == 0 {
		return mailer.ErrNoRecipients
	}
	for i, a := range msg.To {
		if err := a.Validate(); err != nil {
			return fmt.Errorf("mailer: log invalid to[%d]: %w", i, err)
		}
	}
	for i, a := range msg.Cc {
		if err := a.Validate(); err != nil {
			return fmt.Errorf("mailer: log invalid cc[%d]: %w", i, err)
		}
	}
	for i, a := range msg.Bcc {
		if err := a.Validate(); err != nil {
			return fmt.Errorf("mailer: log invalid bcc[%d]: %w", i, err)
		}
	}

	out := logMessage{
		From:        toLogAddress(msg.From),
		To:          toLogAddresses(msg.To),
		Cc:          toLogAddresses(msg.Cc),
		Bcc:         toLogAddresses(msg.Bcc),
		Subject:     msg.Subject,
		Body:        msg.Body,
		HTML:        msg.HTML,
		Attachments: make([]logAttachment, 0, len(msg.Attachments)),
	}
	for _, a := range msg.Attachments {
		out.Attachments = append(out.Attachments, logAttachment{
			Name:      a.Name,
			Size:      len(a.Content),
			Inline:    a.Inline,
			ContentID: a.ContentID,
		})
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() {
		return mailer.ErrClosed
	}
	data, err := c.codec.Encode(out)
	if err != nil {
		return err
	}
	if _, err := c.w.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// Close shuts down the checker. It is idempotent and always returns nil.
func (c *checker) Close() error {
	c.closed.Store(true)
	return nil
}
