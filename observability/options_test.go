package observability

import (
	"errors"
	"testing"
)

func validOptions() Options {
	return Options{
		ServiceName: "svc",
		SampleRatio: 1,
	}
}

func TestOptions_Validate_valid_passes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts Options
	}{
		{name: "minimal", opts: validOptions()},
		{name: "loopback endpoint insecure", opts: func() Options {
			o := validOptions()
			o.Endpoint = "localhost:4317"
			o.Insecure = true
			return o
		}()},
		{name: "127 loopback insecure", opts: func() Options {
			o := validOptions()
			o.Endpoint = "127.0.0.1:4317"
			o.Insecure = true
			return o
		}()},
		{name: "ipv6 loopback insecure", opts: func() Options {
			o := validOptions()
			o.Endpoint = "[::1]:4317"
			o.Insecure = true
			return o
		}()},
		{name: "secure remote", opts: func() Options {
			o := validOptions()
			o.Endpoint = "collector.example.com:4317"
			return o
		}()},
		{name: "zero ratio", opts: func() Options {
			o := validOptions()
			o.SampleRatio = 0
			return o
		}()},
		{name: "default attr limit", opts: func() Options {
			o := validOptions()
			o.AttrValueLimit = 0
			return o
		}()},
		{name: "custom attr limit", opts: func() Options {
			o := validOptions()
			o.AttrValueLimit = 512
			return o
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.opts.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestOptions_Validate_invalid_returnsInvalidOptionsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts func() Options
	}{
		{name: "empty service name", opts: func() Options {
			o := validOptions()
			o.ServiceName = ""
			return o
		}},
		{name: "control chars in service name", opts: func() Options {
			o := validOptions()
			o.ServiceName = "svc\nbad"
			return o
		}},
		{name: "bad endpoint scheme", opts: func() Options {
			o := validOptions()
			o.Endpoint = "https://host:4317"
			return o
		}},
		{name: "bad endpoint slash", opts: func() Options {
			o := validOptions()
			o.Endpoint = "host:4317/v1"
			return o
		}},
		{name: "bad endpoint query", opts: func() Options {
			o := validOptions()
			o.Endpoint = "host:4317?x=1"
			return o
		}},
		{name: "bad endpoint fragment", opts: func() Options {
			o := validOptions()
			o.Endpoint = "host:4317#frag"
			return o
		}},
		{name: "bad endpoint no port", opts: func() Options {
			o := validOptions()
			o.Endpoint = "host"
			return o
		}},
		{name: "bad endpoint port zero", opts: func() Options {
			o := validOptions()
			o.Endpoint = "host:0"
			return o
		}},
		{name: "bad endpoint unspecified v4", opts: func() Options {
			o := validOptions()
			o.Endpoint = "0.0.0.0:4317"
			return o
		}},
		{name: "bad endpoint unspecified v6", opts: func() Options {
			o := validOptions()
			o.Endpoint = "[::]:4317"
			return o
		}},
		{name: "insecure non-loopback", opts: func() Options {
			o := validOptions()
			o.Endpoint = "collector.example.com:4317"
			o.Insecure = true
			return o
		}},
		{name: "bad ratio negative", opts: func() Options {
			o := validOptions()
			o.SampleRatio = -0.1
			return o
		}},
		{name: "bad ratio above one", opts: func() Options {
			o := validOptions()
			o.SampleRatio = 1.5
			return o
		}},
		{name: "bad headers too many", opts: func() Options {
			o := validOptions()
			o.Headers = make(map[string]string, 33)
			for i := 0; i < 33; i++ {
				o.Headers[string(rune('a'+i%26))+string(rune('0'+i/26))] = "v"
			}
			return o
		}},
		{name: "bad headers empty key", opts: func() Options {
			o := validOptions()
			o.Headers = map[string]string{"": "v"}
			return o
		}},
		{name: "bad headers auth insecure", opts: func() Options {
			o := validOptions()
			o.Endpoint = "localhost:4317"
			o.Insecure = true
			o.Headers = map[string]string{"Authorization": "x"}
			return o
		}},
		{name: "bad attr limit small", opts: func() Options {
			o := validOptions()
			o.AttrValueLimit = 10
			return o
		}},
		{name: "bad attr limit large", opts: func() Options {
			o := validOptions()
			o.AttrValueLimit = 1 << 20
			return o
		}},
		{name: "cert without key", opts: func() Options {
			o := validOptions()
			o.CertFile = "cert.pem"
			return o
		}},
		{name: "key without cert", opts: func() Options {
			o := validOptions()
			o.KeyFile = "key.pem"
			return o
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.opts().Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want ErrInvalidOptions")
			}
			if !errors.Is(err, ErrInvalidOptions) {
				t.Errorf("errors.Is(err, ErrInvalidOptions) = false (err = %v)", err)
			}
			var inv *InvalidOptionsError
			if !errors.As(err, &inv) {
				t.Fatalf("errors.As(err, InvalidOptionsError) = false (err = %T %v)", err, err)
			}
			if inv.Reason == "" {
				t.Error("InvalidOptionsError.Reason empty, want non-empty")
			}
		})
	}
}
