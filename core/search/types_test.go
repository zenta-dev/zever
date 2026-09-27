package search

import (
	"testing"
)

func TestDefaultLimit_value(t *testing.T) {
	t.Parallel()

	if DefaultLimit != 10 {
		t.Errorf("DefaultLimit = %d, want 10", DefaultLimit)
	}
}

func TestDocument_zero_values(t *testing.T) {
	t.Parallel()

	var d Document
	if d.ID != "" {
		t.Errorf("ID = %q, want empty", d.ID)
	}

	if d.Index != "" {
		t.Errorf("Index = %q, want empty", d.Index)
	}

	if d.Content != "" {
		t.Errorf("Content = %q, want empty", d.Content)
	}

	if d.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", d.Metadata)
	}
}

func TestQueryOptions_zero_values(t *testing.T) {
	t.Parallel()

	var o QueryOptions
	if o.Limit != 0 {
		t.Errorf("Limit = %d, want 0", o.Limit)
	}

	if o.Offset != 0 {
		t.Errorf("Offset = %d, want 0", o.Offset)
	}

	if o.Filters != nil {
		t.Errorf("Filters = %v, want nil", o.Filters)
	}
}

func TestResult_zero_values(t *testing.T) {
	t.Parallel()

	var r Result
	if r.Hits != nil {
		t.Errorf("Hits = %v, want nil", r.Hits)
	}

	if r.Total != 0 {
		t.Errorf("Total = %d, want 0", r.Total)
	}
}

func TestHit_zero_values(t *testing.T) {
	t.Parallel()

	var h Hit
	if h.ID != "" {
		t.Errorf("ID = %q, want empty", h.ID)
	}

	if h.Score != 0 {
		t.Errorf("Score = %v, want 0", h.Score)
	}

	if h.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", h.Metadata)
	}
}
