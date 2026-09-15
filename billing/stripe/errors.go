package stripe

import "errors"

// ErrMissingSecretKey is returned when Stripe options carry no secret key.
var ErrMissingSecretKey = errors.New("stripe: secret key is required")
