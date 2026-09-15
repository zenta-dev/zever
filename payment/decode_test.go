package payment

import (
	"errors"
	"testing"
)

func TestLimitDecode_ok(t *testing.T) {
	t.Parallel()

	var v Result

	if err := LimitDecode([]byte(`{"ID":"pay_1","Status":"succeeded","Amount":100,"Currency":"USD"}`), &v, 1<<20); err != nil {
		t.Fatalf("LimitDecode err = %v", err)
	}

	if v.ID != "pay_1" || v.Status != PaymentSucceeded || v.Amount != 100 || v.Currency != "USD" {
		t.Fatalf("LimitDecode value = %+v, want pay_1/succeeded/100/USD", v)
	}
}

func TestLimitDecode_overCap_returnsSizeLimit(t *testing.T) {
	t.Parallel()

	data := []byte(`{"ID":"pay_1"}`)
	var v Result

	err := LimitDecode(data, &v, len(data)-1)
	if !errors.Is(err, ErrWebhookTooLarge) {
		t.Fatalf("LimitDecode err = %v, want ErrWebhookTooLarge", err)
	}

	var sle *SizeLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("err %T is not *SizeLimitError", err)
	}

	if sle.Size != len(data) || sle.Limit != len(data)-1 {
		t.Errorf("carried size/limit = {%d %d}, want {%d %d}", sle.Size, sle.Limit, len(data), len(data)-1)
	}
}

func TestLimitDecode_nonPositiveLimit_returnsSizeLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		limit int
	}{
		{"zero", 0},
		{"negative", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data := []byte(`{}`)
			var v Result

			err := LimitDecode(data, &v, tc.limit)
			if !errors.Is(err, ErrWebhookTooLarge) {
				t.Fatalf("LimitDecode err = %v, want ErrWebhookTooLarge", err)
			}

			var sle *SizeLimitError
			if !errors.As(err, &sle) {
				t.Fatalf("err %T is not *SizeLimitError", err)
			}
		})
	}
}

func TestLimitDecode_invalidJSON_returnsError(t *testing.T) {
	t.Parallel()

	var v Result
	if err := LimitDecode([]byte(`{invalid`), &v, 1<<20); err == nil {
		t.Fatalf("LimitDecode invalid JSON = nil, want error")
	}
}

func TestLimitDecode_truncated_returnsError(t *testing.T) {
	t.Parallel()

	var v Result
	if err := LimitDecode([]byte(`{"ID": "pay_1"`), &v, 1<<20); err == nil {
		t.Fatalf("LimitDecode truncated = nil, want error")
	}
}
