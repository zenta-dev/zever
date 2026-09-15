package i18n

import (
	"errors"
	"testing"
	"time"
)

func TestErrors_typedErrorMessage_exact(t *testing.T) {
	t.Parallel()

	dup := DuplicateError{Adapter: Embed}
	if want := "i18n: duplicate registration: embed"; dup.Error() != want {
		t.Errorf("DuplicateError.Error() = %q, want %q", dup.Error(), want)
	}
	if (&dup).Error() != dup.Error() {
		t.Errorf("DuplicateError pointer receiver = %q, value receiver = %q", (&dup).Error(), dup.Error())
	}

	unk := UnknownAdapterError{Adapter: Remote}
	if want := "i18n: unknown adapter: remote"; unk.Error() != want {
		t.Errorf("UnknownAdapterError.Error() = %q, want %q", unk.Error(), want)
	}
	if (&unk).Error() != unk.Error() {
		t.Errorf("UnknownAdapterError pointer receiver = %q, value receiver = %q", (&unk).Error(), unk.Error())
	}

	inv := InvalidAdapterError{Adapter: "x"}
	if want := `i18n: invalid adapter: "x"`; inv.Error() != want {
		t.Errorf("InvalidAdapterError.Error() = %q, want %q", inv.Error(), want)
	}
	if (&inv).Error() != inv.Error() {
		t.Errorf("InvalidAdapterError pointer receiver = %q, value receiver = %q", (&inv).Error(), inv.Error())
	}

	ioe := InvalidOptionsError{Reason: "<reason>"}
	if want := "i18n: invalid options: <reason>"; ioe.Error() != want {
		t.Errorf("InvalidOptionsError.Error() = %q, want %q", ioe.Error(), want)
	}
	if (&ioe).Error() != ioe.Error() {
		t.Errorf("InvalidOptionsError pointer receiver = %q, value receiver = %q", (&ioe).Error(), ioe.Error())
	}

	lne := LocaleNotFoundError{Locale: "en"}
	if want := "i18n: locale not found: en"; lne.Error() != want {
		t.Errorf("LocaleNotFoundError.Error() = %q, want %q", lne.Error(), want)
	}
	if (&lne).Error() != lne.Error() {
		t.Errorf("LocaleNotFoundError pointer receiver = %q, value receiver = %q", (&lne).Error(), lne.Error())
	}

	kne := KeyNotFoundError{Locale: "en", Key: "hello"}
	if want := "i18n: key not found: en/hello"; kne.Error() != want {
		t.Errorf("KeyNotFoundError.Error() = %q, want %q", kne.Error(), want)
	}
	if (&kne).Error() != kne.Error() {
		t.Errorf("KeyNotFoundError pointer receiver = %q, value receiver = %q", (&kne).Error(), kne.Error())
	}
}

func TestErrors_typedUnwrap_viaErrorValues(t *testing.T) {
	t.Parallel()

	if !errors.Is(DuplicateError{Adapter: Embed}, ErrDuplicate) {
		t.Error("DuplicateError.Error path does not unwrap to ErrDuplicate")
	}
	if !errors.Is(UnknownAdapterError{Adapter: Remote}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError.Error path does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError.Error path does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(InvalidOptionsError{Reason: "<reason>"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError.Error path does not unwrap to ErrInvalidOptions")
	}
	if !errors.Is(LocaleNotFoundError{Locale: "en"}, ErrLocaleNotFound) {
		t.Error("LocaleNotFoundError.Error path does not unwrap to ErrLocaleNotFound")
	}
	if !errors.Is(KeyNotFoundError{Locale: "en", Key: "hello"}, ErrKeyNotFound) {
		t.Error("KeyNotFoundError.Error path does not unwrap to ErrKeyNotFound")
	}
}

func TestOptions_Validate_urlParseFailure_invalid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.Endpoint = "100%"
	err := o.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("url parse failure err = %v, want ErrInvalidOptions", err)
	}
	if want := "i18n: invalid options: endpoint must be a valid URL"; err.Error() != want {
		t.Errorf("url parse failure err %q, want %q", err.Error(), want)
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Errorf("url parse failure err %T is not *InvalidOptionsError", err)
	} else if ioe.Reason != "endpoint must be a valid URL" {
		t.Errorf("url parse failure reason %q", ioe.Reason)
	}
}

func TestOptions_Validate_httpsEndpoint_valid(t *testing.T) {
	t.Parallel()
	o := Options{}
	o.Remote.Endpoint = "https://example.com/x"
	if err := o.Validate(); err != nil {
		t.Errorf("https Validate() = %v, want nil", err)
	}
}

func TestOptions_timeout_defaultAndOverride(t *testing.T) {
	t.Parallel()
	zero := Options{}
	if got := zero.timeout(); got != DefaultTimeout {
		t.Errorf("zero timeout() = %v, want %v", got, DefaultTimeout)
	}
	override := Options{}
	override.Remote.Timeout = 5 * time.Second
	if got := override.timeout(); got != 5*time.Second {
		t.Errorf("explicit timeout() = %v, want %v", got, 5*time.Second)
	}
}
