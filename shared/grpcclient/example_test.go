package grpcclient_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/shared/grpcclient"
)

func ExampleNew() {
	conn, err := grpcclient.New(context.Background(), "static:///users",
		grpcclient.WithInsecure(),
		grpcclient.WithStaticResolver(map[string][]string{
			"users": {"127.0.0.1:50051", "127.0.0.2:50051"},
		}))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer conn.Close()

	fmt.Println("client created")
	// Output: client created
}
