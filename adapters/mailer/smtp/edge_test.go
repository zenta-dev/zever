package smtp

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/mailer"
)

// TestEdgeBuildMIME_emptySubjectAndBody proves an empty subject and body
// still render valid MIME with no alternative body parts.
func TestEdgeBuildMIME_emptySubjectAndBody(t *testing.T) {
	t.Parallel()

	msg := &mailer.Mail{
		From: mailer.Address{Address: "from@example.com"},
		To:   []mailer.Address{{Address: "to@example.com"}},
	}

	data, err := buildMIME(msg)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}

	out := string(data)
	if !strings.Contains(out, "MIME-Version: 1.0") {
		t.Fatalf("MIME missing version header:\n%s", out)
	}

	if strings.Contains(out, "text/plain") || strings.Contains(out, "text/html") {
		t.Fatalf("empty body produced an alternative part:\n%s", out)
	}
}

// TestEdgeEstimateSize_monotonic proves the estimate grows with body length
// and recipient count.
func TestEdgeEstimateSize_monotonic(t *testing.T) {
	t.Parallel()

	base := &mailer.Mail{Subject: "s", Body: "b"}
	small := estimateSize(base, 1)

	if big := estimateSize(&mailer.Mail{Subject: "s", Body: strings.Repeat("b", 1024)}, 1); big <= small {
		t.Fatalf("estimate with larger body = %d, want > %d", big, small)
	}

	if more := estimateSize(base, 10); more <= small {
		t.Fatalf("estimate with more recipients = %d, want > %d", more, small)
	}
}

// TestEdgeEstimateSize_attachmentOverhead proves attachments add base64
// overhead beyond their raw length.
func TestEdgeEstimateSize_attachmentOverhead(t *testing.T) {
	t.Parallel()

	const raw = 3000

	msg := &mailer.Mail{
		Subject:     "s",
		Attachments: []mailer.Attachment{{Name: "a.bin", Content: make([]byte, raw)}},
	}

	if got := estimateSize(msg, 1); got <= raw {
		t.Fatalf("estimate = %d, want > raw %d (base64 overhead)", got, raw)
	}
}
