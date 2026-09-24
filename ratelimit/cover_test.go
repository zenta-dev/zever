package ratelimit

import "testing"

func TestCoverTypedErrorStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"duplicate", (&DuplicateError{Adapter: Redis}).Error(), "ratelimit: duplicate registration: redis"},
		{"unknown", (&UnknownAdapterError{Adapter: Memory}).Error(), "ratelimit: unknown adapter: memory (forgotten import?)"},
		{"invalid_adapter", (&InvalidAdapterError{Adapter: "bogus"}).Error(), `ratelimit: invalid adapter: "bogus"`},
		{"invalid_options", (&InvalidOptionsError{Reason: "rate must be > 0 and finite"}).Error(), "ratelimit: invalid options: rate must be > 0 and finite"},
		{"invalid_key", (&InvalidKeyError{KeyLen: 3}).Error(), "ratelimit: invalid key: invalid length 3"},
	}

	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s Error() = %q, want %q", c.name, c.got, c.want)
		}
	}
}
