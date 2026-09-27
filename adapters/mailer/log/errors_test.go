package log

import (
	"bytes"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/mailer"
)

func TestSendNilMessage(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	m, err := NewWithWriter(mailer.Options{Host: "localhost", Port: 25}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter() error = %v", err)
	}

	if err := m.Send(t.Context(), nil); !errors.Is(err, mailer.ErrNilMessage) {
		t.Errorf("Send(nil) err = %v, want ErrNilMessage", err)
	}
}
