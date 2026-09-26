package codec_test

import (
	"fmt"

	"github.com/zenta-dev/zever/shared/codec"
)

type point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// ExampleJSONCodec encodes a value with the JSON codec and decodes it back.
func ExampleJSONCodec() {
	var c codec.JSONCodec[point]

	raw, err := c.Encode(point{X: 1, Y: 2})
	if err != nil {
		fmt.Println("encode error")
		return
	}

	back, err := c.Decode(raw)
	if err != nil {
		fmt.Println("decode error")
		return
	}

	fmt.Println(string(raw), back == (point{X: 1, Y: 2}))
	// Output: {"x":1,"y":2} true
}
