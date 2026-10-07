package outboxbridge

import "errors"

// ErrNilQueue reports a nil queue handed to QueuePublisher.
var ErrNilQueue = errors.New("outboxbridge: nil queue")

// ErrNilEventBus reports a nil eventbus handed to EventBusPublisher.
var ErrNilEventBus = errors.New("outboxbridge: nil eventbus")
