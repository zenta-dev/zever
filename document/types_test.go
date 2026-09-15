package document

import (
	"testing"
)

func TestOutputFormat_consts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		format OutputFormat
		want   string
	}{
		{"pdf", FormatPDF, "pdf"},
		{"png", FormatPNG, "png"},
		{"jpg", FormatJPG, "jpg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if string(tc.format) != tc.want {
				t.Errorf("format = %q, want %q", string(tc.format), tc.want)
			}
		})
	}
}
