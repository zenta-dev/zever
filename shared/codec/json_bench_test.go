package codec

import "testing"

var benchJSONValue = jsonTestValue{
	Name:  "zever",
	Count: 42,
	Score: 3.14,
	Items: []string{"a", "b"},
	Tags:  map[string]string{"k": "v"},
}

// BenchmarkJSONEncode measures JSONCodec.Encode on a populated struct.
func BenchmarkJSONEncode(b *testing.B) {
	c := JSONCodec[jsonTestValue]{}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, _ = c.Encode(benchJSONValue)
	}
}

// BenchmarkJSONDecode measures JSONCodec.Decode on a populated document.
func BenchmarkJSONDecode(b *testing.B) {
	c := JSONCodec[jsonTestValue]{}

	data, err := c.Encode(benchJSONValue)
	if err != nil {
		b.Fatalf("Encode() setup error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, _ = c.Decode(data)
	}
}

// BenchmarkJSONRoundTrip measures a full encode/decode cycle.
func BenchmarkJSONRoundTrip(b *testing.B) {
	c := JSONCodec[jsonTestValue]{}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		data, err := c.Encode(benchJSONValue)
		if err != nil {
			b.Fatalf("Encode() error = %v", err)
		}

		_, _ = c.Decode(data)
	}
}

// BenchmarkJSONEncodeParallel measures Encode under concurrent callers.
func BenchmarkJSONEncodeParallel(b *testing.B) {
	c := JSONCodec[jsonTestValue]{}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = c.Encode(benchJSONValue)
		}
	})
}
