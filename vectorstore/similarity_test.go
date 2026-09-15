package vectorstore

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		a     []float32
		b     []float32
		want  float32
		delta float32
	}{
		{"identical", []float32{1, 0}, []float32{1, 0}, 1, 1e-6},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0, 1e-6},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1, 1e-6},
		{"length mismatch", []float32{1, 0}, []float32{1}, 0, 0},
		{"empty a", nil, []float32{1}, 0, 0},
		{"empty b", []float32{1}, nil, 0, 0},
		{"both empty", nil, nil, 0, 0},
		{"zero norm a", []float32{0, 0}, []float32{1, 0}, 0, 0},
		{"zero norm b", []float32{1, 0}, []float32{0, 0}, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := CosineSimilarity(tc.a, tc.b)
			if math.Abs(float64(got-tc.want)) > float64(tc.delta) {
				t.Errorf("CosineSimilarity = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSqrt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		in    float32
		want  float32
		delta float64
	}{
		{"zero", 0, 0, 0},
		{"negative", -4, 0, 0},
		{"positive", 4, 2, 1e-6},
		{"one", 1, 1, 1e-6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := sqrt(tc.in)
			if math.Abs(float64(got-tc.want)) > tc.delta {
				t.Errorf("sqrt(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
