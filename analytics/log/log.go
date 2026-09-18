package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/zenta-dev/zever/analytics"
)

// adapter renders analytics events as JSON log lines.
type adapter struct {
	// logger receives rendered events.
	logger *slog.Logger
	// anonymousID is the fallback identity.
	anonymousID string
	// maxPropertiesBytes bounds the encoded payload size.
	maxPropertiesBytes int
	// maxProperties bounds the property count.
	maxProperties int
}

// New returns an adapter using o. The default logger writes JSON lines to
// stdout; there is no PII redaction, so use it for debug output only.
// For tests, prefer NewWithWriter with a buffer.
func New(o analytics.Options) (analytics.Analytics, error) {
	return NewWithWriter(o, os.Stdout)
}

// NewWithWriter returns an adapter using o that writes JSON lines to w.
// A nil w falls back to stdout, mirroring log/pretty and
// observability/stdout. The slog JSON handler is safe for concurrent use.
func NewWithWriter(o analytics.Options, w io.Writer) (analytics.Analytics, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}

	anonymousID := o.AnonymousID
	if anonymousID == "" {
		anonymousID = analytics.DefaultAnonymousID
	}

	maxBytes := o.MaxPropertiesBytes
	if maxBytes <= 0 {
		maxBytes = analytics.DefaultMaxPropertiesBytes
	}

	if w == nil {
		w = os.Stdout
	}

	return &adapter{
		logger:             slog.New(slog.NewJSONHandler(w, nil)),
		anonymousID:        anonymousID,
		maxPropertiesBytes: maxBytes,
		maxProperties:      o.MaxProperties,
	}, nil
}

// checkBounds validates the count and encoded size of m.
func (a *adapter) checkBounds(m map[string]any) error {
	if err := analytics.ValidateBounds(m, a.maxProperties, a.maxPropertiesBytes); err != nil {
		return fmt.Errorf("log: %w", err)
	}

	return nil
}

// Track renders event with properties as a JSON log line.
func (a *adapter) Track(ctx context.Context, event string, properties map[string]any) error {
	if err := a.checkBounds(properties); err != nil {
		return err
	}

	props, err := analytics.MarshalValidated(properties, a.maxPropertiesBytes)
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}

	userID := analytics.UserIDWithFallback(ctx, a.anonymousID)
	if userID == "" {
		return fmt.Errorf("log: track: %w", analytics.ErrMissingIdentity)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	a.logger.InfoContext(ctx, "track", "user_id", userID, "event", event, "properties", string(props))

	return nil
}

// Identify renders traits for userID as a JSON log line.
func (a *adapter) Identify(ctx context.Context, userID string, traits map[string]any) error {
	if err := a.checkBounds(traits); err != nil {
		return err
	}

	t, err := analytics.MarshalValidated(traits, a.maxPropertiesBytes)
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}

	if userID == "" {
		userID = analytics.UserIDWithFallback(ctx, a.anonymousID)
	}

	if userID == "" {
		return fmt.Errorf("log: identify: %w", analytics.ErrMissingIdentity)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	a.logger.InfoContext(ctx, "identify", "user_id", userID, "traits", string(t))

	return nil
}

// Group renders the user-to-group association as a JSON log line.
func (a *adapter) Group(ctx context.Context, userID string, groupID string, traits map[string]any) error {
	if err := a.checkBounds(traits); err != nil {
		return err
	}

	t, err := analytics.MarshalValidated(traits, a.maxPropertiesBytes)
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}

	if userID == "" {
		userID = analytics.UserIDWithFallback(ctx, a.anonymousID)
	}

	if userID == "" {
		return fmt.Errorf("log: group: %w", analytics.ErrMissingIdentity)
	}

	if groupID == "" {
		return fmt.Errorf("log: group: %w", analytics.ErrMissingGroupID)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	a.logger.InfoContext(ctx, "group", "user_id", userID, "group_id", groupID, "traits", string(t))

	return nil
}

// Close releases adapter resources.
func (a *adapter) Close() error {
	return nil
}
