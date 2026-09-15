package paddle

import "errors"

// ErrMissingAPIKey is returned when Paddle options carry no API key.
var ErrMissingAPIKey = errors.New("paddle: api key is required")
