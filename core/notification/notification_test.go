package notification

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var freshSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(fmt.Sprintf("test-%d", 1000+int(freshSeq.Add(1))))
}

type stubNotifier struct {
	notified *Notification
	err      error
}

func (s *stubNotifier) Notify(_ context.Context, n *Notification) error {
	s.notified = n
	return s.err
}

func (s *stubNotifier) Close() error { return nil }

func TestRegister_nilFactory_returnsErrNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (Notifier, error) { return &stubNotifier{}, nil }
	if err := Register(a, ok); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	if err := Register(a, ok); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second Register err = %v, want ErrDuplicate", err)
	}
	var de *DuplicateError
	if err := Register(a, ok); !errors.As(err, &de) {
		t.Fatalf("dup err %T is not *DuplicateError", err)
	} else if de.Adapter != a {
		t.Fatalf("dup carried adapter = %v want %v", de.Adapter, a)
	}
}

func TestOpen_unknownAdapter_returnsUnknownError(t *testing.T) {
	a := Adapter("test-9999")
	_, err := Open(a, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}
	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("carried adapter = %v want %v", ue.Adapter, a)
	}
}

func TestOpen_factoryError_wrappedWithAdapter(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (Notifier, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "notification: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "notification: open")
	}
}

func TestOpen_success_returnsNotifier(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Notifier, error) { return &stubNotifier{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	n, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestNewNotification_setsFields(t *testing.T) {
	t.Parallel()
	n := NewNotification("token-123", ChannelPush, "hello")
	if n.Target != "token-123" {
		t.Errorf("Target = %q want %q", n.Target, "token-123")
	}
	if n.Channel != ChannelPush {
		t.Errorf("Channel = %q want %q", n.Channel, ChannelPush)
	}
	if n.Body != "hello" {
		t.Errorf("Body = %q want %q", n.Body, "hello")
	}
	if n.Priority != "" {
		t.Errorf("Priority = %q want zero value", n.Priority)
	}
	if n.TTL != 0 {
		t.Errorf("TTL = %v want 0", n.TTL)
	}
}

func TestNotification_Clone_deepCopy(t *testing.T) {
	t.Parallel()
	orig := Notification{
		Target:   "t",
		Channel:  ChannelPush,
		Title:    "title",
		Body:     "body",
		Data:     map[string]string{"k": "v"},
		Priority: PriorityHigh,
		TTL:      time.Minute,
	}
	cl := orig.Clone()
	if cl.Target != orig.Target || cl.Channel != orig.Channel || cl.Title != orig.Title || cl.Body != orig.Body || cl.Priority != orig.Priority || cl.TTL != orig.TTL {
		t.Fatalf("clone scalars differ: %+v vs %+v", cl, orig)
	}
	cl.Data["k"] = "changed"
	if orig.Data["k"] == "changed" {
		t.Error("Clone Data shares backing map")
	}
}

func TestNotification_Clone_nilData_staysNil(t *testing.T) {
	t.Parallel()
	orig := NewNotification("t", ChannelPush, "b")
	cl := orig.Clone()
	if cl.Data != nil {
		t.Errorf("Clone Data = %v want nil", cl.Data)
	}
}

func TestNotification_Validate_table(t *testing.T) {
	t.Parallel()
	validPush := Notification{Target: "device-token-abc", Channel: ChannelPush, Body: "hi"}
	validSMS := Notification{Target: "+14155552671", Channel: ChannelSMS, Body: "hi"}
	validPushOpts := Notification{Target: "tok", Channel: ChannelPush, Title: "t", Body: "b", Data: map[string]string{"k": "v"}, Priority: PriorityLow, TTL: time.Second}
	validEmptyPriority := Notification{Target: "tok", Channel: ChannelPush, Body: "b", Priority: ""}

	cases := []struct {
		name    string
		n       Notification
		wantErr error // nil = valid
	}{
		{"valid push", validPush, nil},
		{"valid sms", validSMS, nil},
		{"valid push with title data priority ttl", validPushOpts, nil},
		{"valid empty priority", validEmptyPriority, nil},
		{"valid normal priority", Notification{Target: "tok", Channel: ChannelPush, Body: "b", Priority: PriorityNormal}, nil},
		{"valid high priority", Notification{Target: "tok", Channel: ChannelPush, Body: "b", Priority: PriorityHigh}, nil},
		{"valid zero ttl", Notification{Target: "tok", Channel: ChannelPush, Body: "b"}, nil},
		{"empty target push", Notification{Channel: ChannelPush, Body: "b"}, ErrInvalidTarget},
		{"empty target sms", Notification{Target: "", Channel: ChannelSMS, Body: "b"}, ErrInvalidTarget},
		{"bad E.164 missing plus", Notification{Target: "4155552671", Channel: ChannelSMS, Body: "b"}, ErrInvalidTarget},
		{"bad E.164 leading zero", Notification{Target: "+04155552671", Channel: ChannelSMS, Body: "b"}, ErrInvalidTarget},
		{"bad E.164 too short", Notification{Target: "+12345", Channel: ChannelSMS, Body: "b"}, ErrInvalidTarget},
		{"bad E.164 too long", Notification{Target: "+1234567890123456", Channel: ChannelSMS, Body: "b"}, ErrInvalidTarget},
		{"bad E.164 letters", Notification{Target: "+1415abc2671", Channel: ChannelSMS, Body: "b"}, ErrInvalidTarget},
		{"unknown channel", Notification{Target: "t", Channel: "email", Body: "b"}, ErrInvalidChannel},
		{"empty channel", Notification{Target: "t", Body: "b"}, ErrInvalidChannel},
		{"uppercase channel", Notification{Target: "t", Channel: "PUSH", Body: "b"}, ErrInvalidChannel},
		{"bad priority", Notification{Target: "t", Channel: ChannelPush, Body: "b", Priority: "urgent"}, ErrInvalidNotification},
		{"negative TTL", Notification{Target: "t", Channel: ChannelPush, Body: "b", TTL: -time.Second}, ErrInvalidNotification},
		{"Title on sms", Notification{Target: "+14155552671", Channel: ChannelSMS, Title: "t", Body: "b"}, ErrInvalidNotification},
		{"Data on sms", Notification{Target: "+14155552671", Channel: ChannelSMS, Body: "b", Data: map[string]string{"k": "v"}}, ErrInvalidNotification},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := c.n.Validate()
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, c.wantErr)
			}
		})
	}
}
