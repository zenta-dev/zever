package vectorstore

import "testing"

func TestVector_zeroValues(t *testing.T) {
	t.Parallel()

	var v Vector
	if v.ID != "" {
		t.Errorf("ID = %q, want empty", v.ID)
	}

	if v.Embedding != nil {
		t.Errorf("Embedding = %v, want nil", v.Embedding)
	}

	if v.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", v.Metadata)
	}
}

func TestScoreMatch_zeroValues(t *testing.T) {
	t.Parallel()

	var m ScoreMatch
	if m.ID != "" {
		t.Errorf("ID = %q, want empty", m.ID)
	}

	if m.Score != 0 {
		t.Errorf("Score = %v, want 0", m.Score)
	}

	if m.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", m.Metadata)
	}
}
