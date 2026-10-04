package payment

import (
	"errors"
	"testing"
)

func TestLimitDecode_limitEqualsLen_succeeds(t *testing.T) {
	t.Parallel()

	data := []byte(`{"ID":"pay_1","Amount":100}`)
	var v Result

	if err := LimitDecode(data, &v, len(data)); err != nil {
		t.Fatalf("LimitDecode(limit==len) err = %v, want nil", err)
	}
	if v.ID != "pay_1" || v.Amount != 100 {
		t.Fatalf("LimitDecode value = %+v, want pay_1/100", v)
	}
}

func TestLimitDecode_emptyData_returnsDecodeErrorNotSizeError(t *testing.T) {
	t.Parallel()

	var v Result

	err := LimitDecode(nil, &v, DefaultMaxWebhookBytes)
	if err == nil {
		t.Fatal("LimitDecode(nil) err = nil, want decode error")
	}
	if errors.Is(err, ErrWebhookTooLarge) {
		t.Fatalf("LimitDecode(nil) err = %v, want decode error not ErrWebhookTooLarge", err)
	}
}

func TestOpen_invalidOptions_checkedBeforeLookup(t *testing.T) {
	t.Parallel()

	_, err := Open(Adapter("edge-unknown"), Options{MaxWebhookBytes: -1})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open(invalid opts, unknown adapter) err = %v, want ErrInvalidOptions", err)
	}
}
