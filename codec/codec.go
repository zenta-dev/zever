package codec

// Encoder serializes a value of type V into bytes.
type Encoder[V any] interface {
	// Encode converts v into its encoded byte representation.
	Encode(v V) ([]byte, error)
}

// Decoder deserializes a value of type V from bytes.
type Decoder[V any] interface {
	// Decode reconstructs a value of type V from its encoded byte representation.
	Decode(data []byte) (V, error)
}

// Codec combines Encoder and Decoder for symmetric round-trip conversion of values of type V.
type Codec[V any] interface {
	Encoder[V]
	Decoder[V]
}
