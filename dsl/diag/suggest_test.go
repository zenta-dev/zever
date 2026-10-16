package diag

import (
	"strings"
	"testing"
)

func TestClosest(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		candidates []string
		want       string
	}{
		{"exact scalar typo", "strnig", []string{"string", "int32", "bool"}, "string"},
		{"transposition", "valdiate", []string{"validate", "default"}, "validate"},
		{"prefix bonus", "valid", []string{"validate", "default"}, "validate"},
		{"case insensitive", "STRING", []string{"string", "bool"}, "string"},
		{"nothing close", "zzz", []string{"string", "bool"}, ""},
		{"empty input", "", []string{"string"}, ""},
		{"no candidates", "string", nil, ""},
		{"short input needs distance one", "ab", []string{"string"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Closest(tt.input, tt.candidates); got != tt.want {
				t.Errorf("Closest(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHintSuffix(t *testing.T) {
	t.Run("formats did-you-mean", func(t *testing.T) {
		got := HintSuffix("strnig", []string{"string", "bool"})
		if got != ` (did you mean "string"?)` {
			t.Fatalf("HintSuffix = %q", got)
		}
	})

	t.Run("empty without match", func(t *testing.T) {
		if got := HintSuffix("zzz", []string{"string"}); got != "" {
			t.Fatalf("HintSuffix = %q, want empty", got)
		}
	})

	t.Run("appends cleanly to messages", func(t *testing.T) {
		msg := "unknown field type \"strnig\"" + HintSuffix("strnig", []string{"string"})
		if !strings.Contains(msg, `(did you mean "string"?)`) {
			t.Fatalf("msg = %q", msg)
		}
	})
}
