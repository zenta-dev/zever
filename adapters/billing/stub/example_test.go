package stub_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/adapters/billing/stub"
	"github.com/zenta-dev/zever/core/billing"
)

// ExampleOpen registers the stub backend, opens it and creates a customer.
func ExampleOpen() {
	_ = billing.Register(billing.Stub, stub.Open)

	backend, err := billing.Open(billing.Stub, billing.Options{})
	if err != nil {
		fmt.Println("open error")
		return
	}
	defer backend.Close()

	customer, err := backend.CreateCustomer(context.Background(), "Ada", "ada@example.com", "")
	if err != nil {
		fmt.Println("create error")
		return
	}

	fmt.Println(customer.Name, customer.Email != "")
	// Output: Ada true
}
