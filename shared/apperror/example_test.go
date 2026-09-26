package apperror_test

import (
	"fmt"

	"github.com/zenta-dev/zever/apperror"
)

// ExampleNew builds a typed error and maps it to an HTTP status.
func ExampleNew() {
	err := apperror.New(apperror.NotFound, "invoice missing")

	fmt.Println(err.Code(), err.Code().HTTPStatus())
	// Output: NOT_FOUND 404
}
