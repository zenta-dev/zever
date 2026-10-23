package stub_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/adapters/billing/stub"
)

// Example opens the stub backend and creates a customer.
func Example() {
	backend := stub.New()
	defer backend.Close()

	customer, err := backend.CreateCustomer(context.Background(), "Ada", "ada@example.com", "")
	if err != nil {
		fmt.Println("create error")
		return
	}

	fmt.Println(customer.Name, customer.Email != "")
	// Output: Ada true
}
