package eventbus_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/adapters/eventbus/memory"
	"github.com/zenta-dev/zever/core/eventbus"
)

// ExampleOpen opens the memory bus and receives a published message.
func ExampleOpen() {
	if err := eventbus.Register(eventbus.Memory, memory.New); err != nil {
		return
	}

	bus, err := eventbus.Open(eventbus.Memory, eventbus.Options{})
	if err != nil {
		return
	}

	defer func() { _ = bus.Close() }()

	ctx := context.Background()

	ch, err := bus.SubscribeChan(ctx, "orders", 16)
	if err != nil {
		return
	}

	defer func() { _ = bus.Unsubscribe("orders", ch) }()

	if err := bus.Publish(ctx, "orders", eventbus.NewPayload([]byte("hello")), nil); err != nil {
		return
	}

	msg := <-ch
	fmt.Println(string(msg.Payload))
	// Output: hello
}
