package mailer

import (
	"errors"
	"testing"
	"time"
)

func TestOptions_Validate_zero_valid(t *testing.T) {
	t.Parallel()
	o := Options{Host: "smtp.example.com", Port: 587}
	if err := o.Validate(); err != nil {
		t.Fatalf("zero Validate() = %v, want nil", err)
	}
	if o.encryption() != EncryptionSTARTTLS {
		t.Errorf("encryption() = %q want starttls", o.encryption())
	}
	if o.timeout() != DefaultTimeout {
		t.Errorf("timeout() = %v want %v", o.timeout(), DefaultTimeout)
	}
	if o.maxMessageSize() != DefaultMaxMessageSize {
		t.Errorf("maxMessageSize() = %v want %v", o.maxMessageSize(), DefaultMaxMessageSize)
	}
}

func TestOptions_Validate_emptyHost(t *testing.T) {
	t.Parallel()
	for _, h := range []string{"", "   ", "\t\n "} {
		o := Options{Host: h, Port: 587}
		if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("host %q err = %v, want ErrInvalidOptions", h, err)
		}
	}
}

func TestOptions_Validate_hostWithScheme(t *testing.T) {
	t.Parallel()
	o := Options{Host: "smtp://smtp.example.com", Port: 587}
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_badPort(t *testing.T) {
	t.Parallel()
	for _, p := range []int{0, -1, 65536, 99999} {
		o := Options{Host: "smtp.example.com", Port: p}
		if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("port %d err = %v, want ErrInvalidOptions", p, err)
		}
	}
}

func TestOptions_Validate_halfCreds(t *testing.T) {
	t.Parallel()
	cases := []Options{
		{Host: "h.example.com", Port: 587, Username: "u"},
		{Host: "h.example.com", Port: 587, Password: "p"},
	}
	for _, o := range cases {
		if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("half creds %+v err = %v, want ErrInvalidOptions", o, err)
		}
	}
}

func TestOptions_Validate_badEncryption(t *testing.T) {
	t.Parallel()
	o := Options{Host: "h.example.com", Port: 587, Encryption: Encryption("tls")}
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_noneWithCreds(t *testing.T) {
	t.Parallel()
	o := Options{Host: "h.example.com", Port: 25, Username: "u", Password: "p", Encryption: EncryptionNone}
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Validate_negativeTimeoutSize(t *testing.T) {
	t.Parallel()
	o := Options{Host: "h.example.com", Port: 587, Timeout: -time.Second}
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("negative timeout err = %v, want ErrInvalidOptions", err)
	}
	o = Options{Host: "h.example.com", Port: 587, MaxMessageSize: -1}
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("negative size err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_defaults_passthrough(t *testing.T) {
	t.Parallel()
	o := Options{Host: "h.example.com", Port: 587, Encryption: EncryptionImplicitTLS, Timeout: 5 * time.Second, MaxMessageSize: 1024}
	if o.encryption() != EncryptionImplicitTLS {
		t.Errorf("encryption() = %q", o.encryption())
	}
	if o.timeout() != 5*time.Second {
		t.Errorf("timeout() = %v", o.timeout())
	}
	if o.maxMessageSize() != 1024 {
		t.Errorf("maxMessageSize() = %v", o.maxMessageSize())
	}
}
