package rag

// chunkText splits text into overlapping chunks of at most size runes. It
// returns nil for empty text and never splits a multi-byte rune. An overlap
// that is negative or not smaller than size is clamped to half of size.
func chunkText(text string, size, overlap int) []string {
	if text == "" {
		return nil
	}

	if size <= 0 {
		size = DefaultChunkSize
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 2
	}

	runes := []rune(text)

	var chunks []string
	for start := 0; start < len(runes); {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}

		chunks = append(chunks, string(runes[start:end]))

		if end == len(runes) {
			break
		}

		start = end - overlap
	}

	return chunks
}
