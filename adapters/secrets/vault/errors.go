package vault

import "errors"

// ErrInvalidBase64 is returned when a stored secret value is not valid
// base64. Set always base64-encodes, so a raw value indicates a foreign
// or corrupted writer; Get fails closed instead of returning it verbatim.
var ErrInvalidBase64 = errors.New("vault: stored value is not valid base64")
