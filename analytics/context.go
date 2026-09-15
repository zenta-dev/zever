package analytics

import (
	"context"
)

type userIDKey struct{}

// WithUserID stores a user ID in the context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

// UserID extracts the user ID from context, returning "" when absent.
func UserID(ctx context.Context) string {
	id, _ := ctx.Value(userIDKey{}).(string)

	return id
}

// UserIDWithFallback returns UserID(ctx) when present, otherwise fallback.
func UserIDWithFallback(ctx context.Context, fallback string) string {
	if id := UserID(ctx); id != "" {
		return id
	}

	return fallback
}
