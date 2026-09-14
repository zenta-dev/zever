package smtp

import "errors"

// ErrNilMessage is returned when a send message is nil.
var ErrNilMessage = errors.New("smtp: nil message")

// ErrSTARTTLSRequired is returned when the server does not advertise STARTTLS.
var ErrSTARTTLSRequired = errors.New("smtp: STARTTLS not advertised by server")
