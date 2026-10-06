package grpcclient

import "errors"

// ErrNoCredentials is returned by New when neither WithInsecure nor WithTLS
// was provided. Credentials are explicit: the default is fail-closed.
var ErrNoCredentials = errors.New("grpcclient: no credentials: pass WithInsecure or WithTLS")

// ErrInsecureAndTLS is returned by New when both WithInsecure and WithTLS
// were provided; the two options are mutually exclusive.
var ErrInsecureAndTLS = errors.New("grpcclient: cannot combine WithInsecure and WithTLS")
