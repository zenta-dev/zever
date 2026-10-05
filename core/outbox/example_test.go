package outbox_test

import (
	"fmt"

	"github.com/zenta-dev/zever/core/outbox"
)

func ExampleMessage() {
	msg := outbox.Message{ID: "evt-1", Topic: "orders", Payload: []byte("{}")}

	if err := msg.Validate(); err != nil {
		fmt.Println("invalid:", err)

		return
	}

	fmt.Println(msg.Topic)
	// Output: orders
}
