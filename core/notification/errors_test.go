package notification

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinel_messages(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"ErrClosed":              {ErrClosed.Error(), "notification: closed"},
		"ErrNilFactory":          {ErrNilFactory.Error(), "notification: nil factory"},
		"ErrDuplicate":           {ErrDuplicate.Error(), "notification: duplicate adapter"},
		"ErrUnknownAdapter":      {ErrUnknownAdapter.Error(), "notification: unknown adapter"},
		"ErrInvalidAdapter":      {ErrInvalidAdapter.Error(), "notification: invalid adapter"},
		"ErrInvalidOptions":      {ErrInvalidOptions.Error(), "notification: invalid options"},
		"ErrNilNotification":     {ErrNilNotification.Error(), "notification: nil notification"},
		"ErrInvalidNotification": {ErrInvalidNotification.Error(), "notification: invalid notification"},
		"ErrInvalidTarget":       {ErrInvalidTarget.Error(), "notification: invalid target"},
		"ErrInvalidChannel":      {ErrInvalidChannel.Error(), "notification: invalid channel"},
		"ErrChannelNotSupported": {ErrChannelNotSupported.Error(), "notification: channel not supported"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q want %q", name, c[0], c[1])
		}
		if !strings.HasPrefix(c[0], "notification: ") {
			t.Errorf("%s %q missing %q prefix", name, c[0], "notification: ")
		}
	}
}

func TestErrors_typed_unwrap(t *testing.T) {
	t.Parallel()
	if !errors.Is(DuplicateAdapterError{Adapter: "log"}, ErrDuplicate) {
		t.Error("DuplicateAdapterError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(UnknownAdapterError{Adapter: "log"}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
	if !errors.Is(InvalidNotificationError{Reason: "x"}, ErrInvalidNotification) {
		t.Error("InvalidNotificationError does not unwrap to ErrInvalidNotification")
	}
	if !errors.Is(InvalidTargetError{Reason: "x"}, ErrInvalidTarget) {
		t.Error("InvalidTargetError does not unwrap to ErrInvalidTarget")
	}
	if !errors.Is(InvalidChannelError{Channel: "x"}, ErrInvalidChannel) {
		t.Error("InvalidChannelError does not unwrap to ErrInvalidChannel")
	}
	if !errors.Is(ChannelNotSupportedError{Channel: ChannelPush}, ErrChannelNotSupported) {
		t.Error("ChannelNotSupportedError does not unwrap to ErrChannelNotSupported")
	}
}

func TestErrors_InvalidTargetError_noPIIEcho(t *testing.T) {
	t.Parallel()
	target := "+14155552671"
	err := (InvalidTargetError{Reason: "target must be E.164"}).Error()
	if strings.Contains(err, target) {
		t.Errorf("InvalidTargetError leaks target %q in %q", target, err)
	}
	n := Notification{Target: "", Channel: ChannelSMS, Body: "b"}
	verr := n.Validate()
	if verr == nil {
		t.Fatal("Validate() = nil, want error")
	}
	if strings.Contains(verr.Error(), "+") {
		t.Errorf("Validate error leaks PII: %q", verr.Error())
	}
	// Malformed target must not be echoed either.
	bad := Notification{Target: "not-a-number", Channel: ChannelSMS, Body: "b"}
	verr = bad.Validate()
	if verr == nil {
		t.Fatal("Validate() = nil, want error")
	}
	if strings.Contains(verr.Error(), "not-a-number") {
		t.Errorf("Validate error echoes target: %q", verr.Error())
	}
}
