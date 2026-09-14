package notification

import (
	"testing"
)

func TestCover_Adapter_TwilioString(t *testing.T) {
	t.Parallel()
	if got := Twilio.String(); got != "twilio" {
		t.Errorf("Twilio.String() = %q, want %q", got, "twilio")
	}
}

func TestCover_ParseAdapter_Twilio(t *testing.T) {
	t.Parallel()
	got, err := ParseAdapter("twilio")
	if err != nil {
		t.Fatalf("ParseAdapter(twilio) err = %v", err)
	}
	if got != Twilio {
		t.Fatalf("ParseAdapter(twilio) = %v, want Twilio", got)
	}
	if got.String() != "twilio" {
		t.Errorf("roundtrip String() = %q, want %q", got.String(), "twilio")
	}
}

func TestCover_Errors_ErrorStrings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"DuplicateError", DuplicateError{Adapter: Log}.Error(), "notification: duplicate registration: log"},
		{"UnknownAdapterError", UnknownAdapterError{Adapter: Log}.Error(), "notification: unknown adapter: log"},
		{"InvalidAdapterError", InvalidAdapterError{Adapter: "bogus"}.Error(), `notification: invalid adapter: "bogus"`},
		{"InvalidOptionsError", InvalidOptionsError{Reason: "timeout must be >= 0"}.Error(), "notification: invalid options: timeout must be >= 0"},
		{"InvalidNotificationError", InvalidNotificationError{Reason: "ttl must be >= 0"}.Error(), "notification: invalid notification: ttl must be >= 0"},
		{"InvalidChannelError", InvalidChannelError{Channel: "email"}.Error(), "notification: invalid channel: email"},
		{"ChannelNotSupportedError", ChannelNotSupportedError{Channel: ChannelSMS}.Error(), "notification: channel not supported: sms"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.got != c.want {
				t.Errorf("%s.Error() = %q, want %q", c.name, c.got, c.want)
			}
		})
	}
}
