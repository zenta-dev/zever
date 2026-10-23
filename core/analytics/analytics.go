package analytics

import (
	"context"
)

// Analytics defines the event-tracking contract for analytics backends.
type Analytics interface {
	// Track records an event with optional properties.
	Track(ctx context.Context, event string, properties map[string]any) error
	// Identify associates traits with a user ID.
	Identify(ctx context.Context, userID string, traits map[string]any) error
	// Group associates a user with a group and its traits.
	Group(ctx context.Context, userID string, groupID string, traits map[string]any) error
	// Close flushes buffered events and releases backend resources.
	// Callers must call Close before process exit to avoid dropping events.
	Close() error
}

// Factory creates an Analytics from the given Options.
type Factory func(opts Options) (Analytics, error)
