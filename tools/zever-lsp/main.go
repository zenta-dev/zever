package main

import (
	"context"
	"os"
)

func main() {
	os.Exit(run(context.Background(), os.Stdin, os.Stdout))
}
