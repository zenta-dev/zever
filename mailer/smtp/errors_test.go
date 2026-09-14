package smtp

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/mailer"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want string
	}{
		{ErrNilMessage, "smtp: nil message"},
		{ErrSTARTTLSRequired, "smtp: STARTTLS not advertised by server"},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
}

func TestSendNilMessage(t *testing.T) {
	t.Parallel()
	m, err := New(mailer.Options{
		Host:       "127.0.0.1",
		Port:       25,
		Encryption: mailer.EncryptionNone,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = m.Close() }()
	err = m.Send(context.Background(), nil)
	if err == nil {
		t.Fatal("expected nil-message error, got nil")
	}
	if !errors.Is(err, ErrNilMessage) {
		t.Errorf("got %q want %q", err.Error(), ErrNilMessage.Error())
	}
}
