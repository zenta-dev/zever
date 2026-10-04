package embedded

import "errors"

// ErrInvalidSpecLength indicates invalid spec length.
var ErrInvalidSpecLength = errors.New("embedded: spec length must be 1-256")
