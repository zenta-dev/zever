package posthog

import (
	"context"
	"fmt"
	"net"
	"net/url"

	posthog "github.com/posthog/posthog-go"

	"github.com/zenta-dev/zever/analytics"
)

// client is the minimal seam the adapter needs from the PostHog SDK.
// The real SDK client satisfies it and fakes implement it trivially.
type client interface {
	Enqueue(posthog.Message) error
	Close() error
}

// newClient constructs the SDK client. It is a variable so tests can stub construction.
var newClient = posthog.NewWithConfig

type adapter struct {
	client             client
	anonymousID        string
	groupType          string
	maxPropertiesBytes int
	maxProperties      int
}

// New creates an Analytics backed by PostHog.
// Only Endpoint overrides the SDK config; BatchSize, Interval and
// MaxQueueSize keep the SDK smart defaults for batched background delivery.
func New(o analytics.Options) (analytics.Analytics, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("posthog: %w", err)
	}

	if o.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	if err := checkEndpoint(o.Endpoint); err != nil {
		return nil, err
	}

	cfg := posthog.Config{}
	if o.Endpoint != "" {
		cfg.Endpoint = o.Endpoint
	}

	c, err := newClient(o.APIKey, cfg)
	if err != nil {
		return nil, fmt.Errorf("posthog: %w", err)
	}

	anonymousID := o.AnonymousID
	if anonymousID == "" {
		anonymousID = analytics.DefaultAnonymousID
	}

	groupType := o.GroupType
	if groupType == "" {
		groupType = analytics.DefaultGroupType
	}

	maxBytes := o.MaxPropertiesBytes
	if maxBytes <= 0 {
		maxBytes = analytics.DefaultMaxPropertiesBytes
	}

	return &adapter{
		client:             c,
		anonymousID:        anonymousID,
		groupType:          groupType,
		maxPropertiesBytes: maxBytes,
		maxProperties:      o.MaxProperties,
	}, nil
}

// checkEndpoint rejects non-https endpoints except localhost http for tests.
func checkEndpoint(endpoint string) error {
	if endpoint == "" {
		return nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return &analytics.InvalidOptionsError{Reason: "endpoint must use https or localhost http"}
	}

	if u.Scheme == "https" {
		return nil
	}

	if u.Scheme == "http" && isLocalhost(u.Hostname()) {
		return nil
	}

	return &analytics.InvalidOptionsError{Reason: "endpoint must use https or localhost http"}
}

// isLocalhost reports whether host is localhost or a loopback IP.
func isLocalhost(host string) bool {
	if host == "localhost" {
		return true
	}

	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}

	return false
}

func (a *adapter) checkBounds(m map[string]any) error {
	if err := analytics.ValidateBounds(m, a.maxProperties, a.maxPropertiesBytes); err != nil {
		return fmt.Errorf("posthog: %w", err)
	}

	return nil
}

// Track records an event with optional properties.
func (a *adapter) Track(ctx context.Context, event string, properties map[string]any) error {
	if err := a.checkBounds(properties); err != nil {
		return err
	}

	// Explicit anonymous fallback – avoids silently hiding auth bugs where context lacks a user ID.
	userID := analytics.UserIDWithFallback(ctx, a.anonymousID)
	if userID == "" {
		return fmt.Errorf("posthog: track: %w", analytics.ErrMissingIdentity)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := a.client.Enqueue(posthog.Capture{
		DistinctId: userID,
		Event:      event,
		Properties: posthog.Properties(properties),
	}); err != nil {
		return fmt.Errorf("posthog: track: %w", err)
	}

	return nil
}

// Identify associates traits with a user ID.
func (a *adapter) Identify(ctx context.Context, userID string, traits map[string]any) error {
	if err := a.checkBounds(traits); err != nil {
		return err
	}

	if userID == "" {
		userID = analytics.UserIDWithFallback(ctx, a.anonymousID)
	}

	if userID == "" {
		return fmt.Errorf("posthog: identify: %w", analytics.ErrMissingIdentity)
	}

	props := posthog.Properties(traits)
	if a.anonymousID != "" && userID != a.anonymousID {
		props = make(posthog.Properties, len(traits)+1)
		for k, v := range traits {
			props[k] = v
		}

		props["$anon_distinct_id"] = a.anonymousID
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := a.client.Enqueue(posthog.Identify{
		DistinctId: userID,
		Properties: props,
	}); err != nil {
		return fmt.Errorf("posthog: identify: %w", err)
	}

	return nil
}

// Group associates a user with a group and its traits.
func (a *adapter) Group(ctx context.Context, userID string, groupID string, traits map[string]any) error {
	if err := a.checkBounds(traits); err != nil {
		return err
	}

	if userID == "" {
		userID = analytics.UserIDWithFallback(ctx, a.anonymousID)
	}

	if userID == "" {
		return fmt.Errorf("posthog: group: %w", analytics.ErrMissingIdentity)
	}

	if groupID == "" {
		return fmt.Errorf("posthog: group: %w", analytics.ErrMissingGroupID)
	}

	groupType := a.groupType
	if groupType == "" {
		groupType = analytics.DefaultGroupType
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := a.client.Enqueue(posthog.GroupIdentify{
		Type:       groupType,
		Key:        groupID,
		DistinctId: userID,
		Properties: posthog.Properties(traits),
	}); err != nil {
		return fmt.Errorf("posthog: group: %w", err)
	}

	return nil
}

// Close flushes buffered events and releases backend resources.
func (a *adapter) Close() error {
	if err := a.client.Close(); err != nil {
		return fmt.Errorf("posthog: close: %w", err)
	}

	return nil
}
