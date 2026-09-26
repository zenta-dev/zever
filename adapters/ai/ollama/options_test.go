package ollama

import (
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    Options
		wantErr bool
		want    []string
	}{
		{name: "zero ok", opts: Options{}},
		{name: "model only ok", opts: Options{Model: "llama3"}},
		{name: "positive timeout ok", opts: Options{Timeout: 5 * time.Second}},
		{
			name:    "negative timeout",
			opts:    Options{Timeout: -time.Second},
			wantErr: true,
			want:    []string{"timeout must be >= 0"},
		},
		{
			name:    "whitespace addr",
			opts:    Options{Addr: "http://local host:11434"},
			wantErr: true,
			want:    []string{"must not contain whitespace"},
		},
		{
			name:    "bad scheme",
			opts:    Options{Addr: "ftp://example.com"},
			wantErr: true,
			want:    []string{"http or https scheme"},
		},
		{
			name:    "no host",
			opts:    Options{Addr: "http://"},
			wantErr: true,
			want:    []string{"must have a host"},
		},
		{
			name:    "user info",
			opts:    Options{Addr: "http://user@example.com"},
			wantErr: true,
			want:    []string{"must not contain user info"},
		},
		{
			name:    "unparseable addr",
			opts:    Options{Addr: "http://[::1"},
			wantErr: true,
			want:    []string{"must be a valid URL"},
		},
		{
			name:    "joined violations",
			opts:    Options{Addr: "ftp://", Timeout: -time.Second},
			wantErr: true,
			want:    []string{"timeout must be >= 0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.opts.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				for _, w := range tt.want {
					if !strings.Contains(err.Error(), w) {
						t.Fatalf("error %q missing %q", err.Error(), w)
					}
				}

				return
			}

			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	t.Parallel()

	if DefaultAddr != "http://localhost:11434" {
		t.Fatalf("DefaultAddr = %q", DefaultAddr)
	}

	if DefaultTimeout != 60*time.Second {
		t.Fatalf("DefaultTimeout = %v", DefaultTimeout)
	}
}
