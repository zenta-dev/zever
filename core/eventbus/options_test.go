package eventbus

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOptions_zero_isValid(t *testing.T) {
	t.Parallel()
	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("zero Options Validate err = %v, want nil", err)
	}
}

func TestOptions_negativeBuffer_isInvalid(t *testing.T) {
	t.Parallel()
	err := Options{BufferSize: -1}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("BufferSize -1 err = %v, want ErrInvalidOptions", err)
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
}

func TestOptions_negativeMaxHandlers_isInvalid(t *testing.T) {
	t.Parallel()
	if err := (Options{MaxHandlers: -1}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("MaxHandlers -1 err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_negativeTimeouts_areInvalid(t *testing.T) {
	t.Parallel()
	if err := (Options{HandlerTimeout: -time.Second}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("HandlerTimeout -1s err = %v, want ErrInvalidOptions", err)
	}
	if err := (Options{CloseTimeout: -time.Second}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("CloseTimeout -1s err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_positiveValues_areValid(t *testing.T) {
	t.Parallel()
	o := Options{
		BufferSize:     10,
		MaxHandlers:    4,
		HandlerTimeout: time.Second,
		CloseTimeout:   time.Second,
	}
	if err := o.Validate(); err != nil {
		t.Fatalf("positive Options Validate err = %v, want nil", err)
	}
}

func TestOptions_onPanic_neverValidated(t *testing.T) {
	t.Parallel()
	o := Options{OnPanic: func(string, Message, any) {}}
	if err := o.Validate(); err != nil {
		t.Fatalf("OnPanic set Validate err = %v, want nil", err)
	}
}

func TestOptions_badRedisAddr_isInvalid(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{
		"redis://localhost:6379",
		"http://localhost:6379",
		"localhost",
		"localhost:0",
		"localhost:99999",
		"localhost:notaport",
		":6379",
		"/tmp/redis.sock",
		"localhost:6379/db?x=1",
	} {
		o := Options{Redis: RedisOptions{Addr: addr}}
		if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("Addr %q err = %v, want ErrInvalidOptions", addr, err)
		}
	}
}

func TestOptions_goodRedisAddr_isValid(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"localhost:6379", "127.0.0.1:6380", "redis:6379", "[::1]:6379"} {
		o := Options{Redis: RedisOptions{Addr: addr}}
		if err := o.Validate(); err != nil {
			t.Errorf("Addr %q err = %v, want nil", addr, err)
		}
	}
}

func TestOptions_oversizePrefix_isInvalid(t *testing.T) {
	t.Parallel()
	o := Options{Redis: RedisOptions{Prefix: strings.Repeat("a", 65)}}
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("65-char prefix err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_badPrefixChars_isInvalid(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"has space", "slash/x", "colon:bad", "semi;bad"} {
		o := Options{Redis: RedisOptions{Prefix: p}}
		if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("Prefix %q err = %v, want ErrInvalidOptions", p, err)
		}
	}
}

func TestOptions_goodPrefix_isValid(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"", "zever-eb", "prod_v1.2", strings.Repeat("a", 64)} {
		o := Options{Redis: RedisOptions{Prefix: p}}
		if err := o.Validate(); err != nil {
			t.Errorf("Prefix %q err = %v, want nil", p, err)
		}
	}
}
