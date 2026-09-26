package paddle

import "errors"

// ErrMissingAPIKey is returned when the Paddle API key is empty.
var ErrMissingAPIKey = errors.New("paddle: api key is required")

// ErrMissingPriceID is returned when a create request has no price_id in meta.
var ErrMissingPriceID = errors.New("paddle: meta price_id is required")

// ErrMultiItemPartial is returned when a partial refund targets multiple line items.
var ErrMultiItemPartial = errors.New("paddle: partial refund requires a single line item")
