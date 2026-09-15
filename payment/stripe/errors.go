package stripe

import "errors"

// ErrMissingSecretKey is returned when Stripe options lack a secret key.
var ErrMissingSecretKey = errors.New("stripe: secret key is required")

// ErrMissingWebhookSecret is returned when Stripe options lack a webhook secret.
var ErrMissingWebhookSecret = errors.New("stripe: webhook secret is required")
