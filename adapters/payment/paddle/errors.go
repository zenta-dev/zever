package paddle

import "errors"

// ErrMissingAPIKey is returned when the Paddle API key is empty.
var ErrMissingAPIKey = errors.New("paddle: api key is required")

// ErrMissingPriceID is returned when a create request has no price_id in meta.
var ErrMissingPriceID = errors.New("paddle: meta price_id is required")

// ErrMultiItemPartial is returned when a partial refund targets multiple line items.
var ErrMultiItemPartial = errors.New("paddle: partial refund requires a single line item")

// ErrAmountOverflow is returned when a Paddle decimal total exceeds the int64
// minor-units range once scaled (the whole part no longer fits after the x100
// shift, or the final add/subtract of the fraction carries past the limit).
var ErrAmountOverflow = errors.New("paddle: amount overflows int64 minor units")
