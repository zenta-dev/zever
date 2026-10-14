package vault

import "errors"

// ErrInvalidBase64 is returned when a stored secret value is not valid
// base64. Set always base64-encodes, so a raw value indicates a foreign
// or corrupted writer; Get fails closed instead of returning it verbatim.
var ErrInvalidBase64 = errors.New("vault: stored value is not valid base64")

// ErrInsecureTokenFilePerms is returned when the token file is readable
// beyond its owner (any group/other permission bit, i.e. broader than
// 0600). Restrict the file to 0600, or explicitly opt out with
// Options.AllowInsecureTokenFilePerms.
var ErrInsecureTokenFilePerms = errors.New("vault: token_file must have permissions 0600 or stricter")
