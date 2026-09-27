package vectorstore

import "math"

// CosineSimilarity returns the cosine similarity of a and b.
// It returns 0 when either vector is empty, lengths mismatch, or a norm is zero.
// Only cosine similarity is supported; no other distance metric is used.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float32

	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (sqrt(normA) * sqrt(normB))
}

// sqrt returns the square root of x as a float32.
// It returns 0 for non-positive inputs.
func sqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}

	return float32(math.Sqrt(float64(x)))
}
