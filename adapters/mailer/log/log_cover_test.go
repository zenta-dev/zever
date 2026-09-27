package log

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/shared/codec"
)

func coverOptions() mailer.Options {
	return mailer.Options{Host: "smtp.example.com", Port: 587}
}

func coverMail() *mailer.Mail {
	return &mailer.Mail{
		From:    mailer.Address{Address: "from@example.com"},
		To:      []mailer.Address{{Address: "to@example.com"}},
		Subject: "cover",
		Body:    "body",
	}
}

func TestCoverNewWithWriterNilWriterUsesStdout(t *testing.T) {
	old := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("Pipe err = %v", pipeErr)
	}
	os.Stdout = w
	restored := false
	defer func() {
		if !restored {
			os.Stdout = old
		}
	}()

	m, newErr := NewWithWriter(coverOptions(), nil)
	if newErr != nil {
		t.Fatalf("NewWithWriter err = %v", newErr)
	}
	defer m.Close()
	if sendErr := m.Send(t.Context(), coverMail()); sendErr != nil {
		t.Fatalf("Send err = %v", sendErr)
	}
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("pipe close err = %v", closeErr)
	}
	os.Stdout = old
	restored = true
	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("ReadAll err = %v", readErr)
	}
	if closeRErr := r.Close(); closeRErr != nil {
		t.Fatalf("reader close err = %v", closeRErr)
	}
	var decoded map[string]any
	if jsonErr := json.Unmarshal(out, &decoded); jsonErr != nil {
		t.Fatalf("stdout output not JSON: %v\n%s", jsonErr, out)
	}
	if decoded["subject"] != "cover" {
		t.Errorf("subject = %v want cover", decoded["subject"])
	}
}

func TestCoverSendCCBCCInvalid(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	m, newErr := NewWithWriter(coverOptions(), &buf)
	if newErr != nil {
		t.Fatalf("NewWithWriter err = %v", newErr)
	}
	defer m.Close()

	ccBad := coverMail()
	ccBad.Cc = []mailer.Address{{Address: "bad-cc"}}
	if err := m.Send(t.Context(), ccBad); !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Errorf("bad Cc err = %v, want ErrInvalidAddress", err)
	} else if !strings.Contains(err.Error(), "cc[0]") {
		t.Errorf("bad Cc err = %q want cc[0] context", err.Error())
	}

	bccBad := coverMail()
	bccBad.Bcc = []mailer.Address{{Address: "bad-bcc"}}
	if err := m.Send(t.Context(), bccBad); !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Errorf("bad Bcc err = %v, want ErrInvalidAddress", err)
	} else if !strings.Contains(err.Error(), "bcc[0]") {
		t.Errorf("bad Bcc err = %q want bcc[0] context", err.Error())
	}
}

func TestCoverSendClosedAfterLock(t *testing.T) {
	oldMax := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(oldMax)
	for range 3 {
		c := &checker{w: io.Discard, codec: codec.JSONCodec[logMessage]{}}
		c.mu.Lock()
		done := make(chan error, 1)
		go func() {
			done <- c.Send(t.Context(), coverMail())
		}()
		runtime.Gosched()
		c.closed.Store(true)
		c.mu.Unlock()
		if sendErr := <-done; !errors.Is(sendErr, mailer.ErrClosed) {
			t.Fatalf("Send err = %v, want ErrClosed", sendErr)
		}
	}
}

func TestCoverToLogAddress(t *testing.T) {
	t.Parallel()
	got := toLogAddress(mailer.Address{Name: "Jane", Address: "jane@example.com"})
	if got.Name != "Jane" || got.Address != "jane@example.com" {
		t.Errorf("toLogAddress = %+v", got)
	}
	bare := toLogAddress(mailer.Address{Address: "a@example.com"})
	if bare.Name != "" || bare.Address != "a@example.com" {
		t.Errorf("bare toLogAddress = %+v", bare)
	}
}

func TestCoverToLogAddresses(t *testing.T) {
	t.Parallel()
	if got := toLogAddresses(nil); len(got) != 0 {
		t.Errorf("nil input len = %d want 0", len(got))
	}
	if got := toLogAddresses([]mailer.Address{}); len(got) != 0 {
		t.Errorf("empty input len = %d want 0", len(got))
	}
	in := []mailer.Address{
		{Name: "A", Address: "a@example.com"},
		{Address: "b@example.com"},
	}
	got := toLogAddresses(in)
	if len(got) != 2 {
		t.Fatalf("len = %d want 2", len(got))
	}
	if got[0].Name != "A" || got[0].Address != "a@example.com" {
		t.Errorf("elem 0 = %+v", got[0])
	}
	if got[1].Name != "" || got[1].Address != "b@example.com" {
		t.Errorf("elem 1 = %+v", got[1])
	}
}
