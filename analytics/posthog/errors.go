package posthog

import "errors"

// ErrMissingAPIKey is returned when Options.APIKey is empty.
var ErrMissingAPIKey = errors.New("posthog: api key is required")
