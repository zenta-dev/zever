package remote

import "errors"

// ErrStatus is returned when the render endpoint replies with a non-200 status.
var ErrStatus = errors.New("remote: unexpected status")
