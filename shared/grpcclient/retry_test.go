package grpcclient

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
)

// validRetryPolicy returns a policy that passes validation.
func validRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:          3,
		InitialBackoff:       10 * time.Millisecond,
		MaxBackoff:           time.Second,
		BackoffMultiplier:    2,
		RetryableStatusCodes: []codes.Code{codes.Unavailable},
	}
}

func TestRetryPolicyValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*RetryPolicy)
		wantErr string
	}{
		{
			name:    "maxAttempts below 2",
			mutate:  func(p *RetryPolicy) { p.MaxAttempts = 1 },
			wantErr: "maxAttempts must be at least 2",
		},
		{
			name:    "zero maxAttempts",
			mutate:  func(p *RetryPolicy) { p.MaxAttempts = 0 },
			wantErr: "maxAttempts must be at least 2",
		},
		{
			name:    "zero initialBackoff",
			mutate:  func(p *RetryPolicy) { p.InitialBackoff = 0 },
			wantErr: "initialBackoff must be positive",
		},
		{
			name:    "negative initialBackoff",
			mutate:  func(p *RetryPolicy) { p.InitialBackoff = -time.Millisecond },
			wantErr: "initialBackoff must be positive",
		},
		{
			name:    "zero maxBackoff",
			mutate:  func(p *RetryPolicy) { p.MaxBackoff = 0 },
			wantErr: "maxBackoff must be positive",
		},
		{
			name:    "zero backoffMultiplier",
			mutate:  func(p *RetryPolicy) { p.BackoffMultiplier = 0 },
			wantErr: "backoffMultiplier must be positive",
		},
		{
			name:    "empty retryableStatusCodes",
			mutate:  func(p *RetryPolicy) { p.RetryableStatusCodes = nil },
			wantErr: "retryableStatusCodes must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := validRetryPolicy()
			tt.mutate(&p)

			err := p.validate()
			if err == nil {
				t.Fatalf("validate succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
			if !strings.HasPrefix(err.Error(), "grpcclient:") {
				t.Fatalf("error = %q, want grpcclient: prefix", err.Error())
			}
		})
	}

	if err := validRetryPolicy().validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
}

// serviceConfigMirror mirrors the gRPC service config JSON shape for
// assertions.
type serviceConfigMirror struct {
	LoadBalancingConfig []map[string]json.RawMessage `json:"loadBalancingConfig"`
	MethodConfig        []struct {
		Name        []map[string]any `json:"name"`
		RetryPolicy *struct {
			MaxAttempts          int      `json:"maxAttempts"`
			InitialBackoff       string   `json:"initialBackoff"`
			MaxBackoff           string   `json:"maxBackoff"`
			BackoffMultiplier    float64  `json:"backoffMultiplier"`
			RetryableStatusCodes []string `json:"retryableStatusCodes"`
		} `json:"retryPolicy"`
	} `json:"methodConfig"`
}

func unmarshalServiceConfig(t *testing.T, raw string) serviceConfigMirror {
	t.Helper()

	var m serviceConfigMirror
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("service config JSON does not unmarshal: %v", err)
	}
	return m
}

func TestServiceConfigJSONDefault(t *testing.T) {
	t.Parallel()

	raw, err := (&config{}).serviceConfigJSON()
	if err != nil {
		t.Fatalf("serviceConfigJSON: %v", err)
	}

	m := unmarshalServiceConfig(t, raw)
	if len(m.LoadBalancingConfig) != 1 {
		t.Fatalf("got %d loadBalancingConfig entries, want 1", len(m.LoadBalancingConfig))
	}
	if _, ok := m.LoadBalancingConfig[0]["round_robin"]; !ok {
		t.Fatalf("loadBalancingConfig = %v, want round_robin", m.LoadBalancingConfig[0])
	}
	if m.MethodConfig != nil {
		t.Fatalf("methodConfig = %v, want none without a retry policy", m.MethodConfig)
	}
}

func TestServiceConfigJSONRetry(t *testing.T) {
	t.Parallel()

	cfg := &config{retry: &RetryPolicy{
		MaxAttempts:          4,
		InitialBackoff:       100 * time.Millisecond,
		MaxBackoff:           2 * time.Second,
		BackoffMultiplier:    1.5,
		RetryableStatusCodes: []codes.Code{codes.Unavailable, codes.ResourceExhausted},
	}}

	raw, err := cfg.serviceConfigJSON()
	if err != nil {
		t.Fatalf("serviceConfigJSON: %v", err)
	}

	m := unmarshalServiceConfig(t, raw)
	if len(m.MethodConfig) != 1 {
		t.Fatalf("got %d methodConfig entries, want 1", len(m.MethodConfig))
	}

	mc := m.MethodConfig[0]
	if len(mc.Name) != 1 || len(mc.Name[0]) != 0 {
		t.Fatalf("methodConfig name = %v, want [{}] (all methods)", mc.Name)
	}
	if mc.RetryPolicy == nil {
		t.Fatal("retryPolicy is nil")
	}
	if mc.RetryPolicy.MaxAttempts != 4 {
		t.Errorf("maxAttempts = %d, want 4", mc.RetryPolicy.MaxAttempts)
	}
	if mc.RetryPolicy.InitialBackoff != "0.100s" {
		t.Errorf("initialBackoff = %q, want %q", mc.RetryPolicy.InitialBackoff, "0.100s")
	}
	if mc.RetryPolicy.MaxBackoff != "2s" {
		t.Errorf("maxBackoff = %q, want %q", mc.RetryPolicy.MaxBackoff, "2s")
	}
	if mc.RetryPolicy.BackoffMultiplier != 1.5 {
		t.Errorf("backoffMultiplier = %v, want 1.5", mc.RetryPolicy.BackoffMultiplier)
	}
	wantCodes := []string{"UNAVAILABLE", "RESOURCE_EXHAUSTED"}
	if len(mc.RetryPolicy.RetryableStatusCodes) != len(wantCodes) {
		t.Fatalf("retryableStatusCodes = %v, want %v", mc.RetryPolicy.RetryableStatusCodes, wantCodes)
	}
	for i, c := range wantCodes {
		if mc.RetryPolicy.RetryableStatusCodes[i] != c {
			t.Errorf("retryableStatusCodes[%d] = %q, want %q", i, mc.RetryPolicy.RetryableStatusCodes[i], c)
		}
	}
}

func TestServiceConfigJSONRetryCapsMaxAttempts(t *testing.T) {
	t.Parallel()

	cfg := &config{retry: &RetryPolicy{
		MaxAttempts:          99,
		InitialBackoff:       time.Millisecond,
		MaxBackoff:           time.Second,
		BackoffMultiplier:    2,
		RetryableStatusCodes: []codes.Code{codes.Unavailable},
	}}

	raw, err := cfg.serviceConfigJSON()
	if err != nil {
		t.Fatalf("serviceConfigJSON: %v", err)
	}

	m := unmarshalServiceConfig(t, raw)
	if got := m.MethodConfig[0].RetryPolicy.MaxAttempts; got != DefaultMaxAttempts {
		t.Fatalf("maxAttempts = %d, want capped at %d", got, DefaultMaxAttempts)
	}
}

func TestServiceConfigJSONRetryInvalidRejected(t *testing.T) {
	t.Parallel()

	cfg := &config{retry: &RetryPolicy{
		MaxAttempts:          1,
		InitialBackoff:       time.Millisecond,
		MaxBackoff:           time.Second,
		BackoffMultiplier:    2,
		RetryableStatusCodes: []codes.Code{codes.Unavailable},
	}}

	_, err := cfg.serviceConfigJSON()
	if err == nil {
		t.Fatal("serviceConfigJSON succeeded for invalid retry policy, want error")
	}
	if !strings.HasPrefix(err.Error(), "grpcclient:") {
		t.Fatalf("error = %q, want grpcclient: prefix", err.Error())
	}
}

func TestServiceConfigJSONMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "replace load balancing",
			raw:  `{"loadBalancingConfig":[{"pick_first":{}}]}`,
			want: "pick_first",
		},
		{
			name: "keep default when key absent",
			raw:  `{"methodConfig":[{"name":[{"service":"x"}]}]}`,
			want: "round_robin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config{serviceConfig: tt.raw, serviceConfigSet: true}
			raw, err := cfg.serviceConfigJSON()
			if err != nil {
				t.Fatalf("serviceConfigJSON: %v", err)
			}

			m := unmarshalServiceConfig(t, raw)
			if len(m.LoadBalancingConfig) != 1 {
				t.Fatalf("got %d loadBalancingConfig entries, want 1", len(m.LoadBalancingConfig))
			}
			for k := range m.LoadBalancingConfig[0] {
				if k != tt.want {
					t.Fatalf("loadBalancingConfig policy = %q, want %q", k, tt.want)
				}
			}
		})
	}
}

func TestNewWithRetryPolicyAcceptedByGRPC(t *testing.T) {
	t.Parallel()

	// grpc.NewClient validates the default service config synchronously,
	// so success proves the emitted retryPolicy parses under gRPC's own
	// parser (including the uppercase status code names).
	conn, err := New(t.Context(), "dns:///localhost:50051",
		WithInsecure(),
		WithRetryPolicy(RetryPolicy{
			MaxAttempts:          3,
			InitialBackoff:       10 * time.Millisecond,
			MaxBackoff:           time.Second,
			BackoffMultiplier:    2,
			RetryableStatusCodes: []codes.Code{codes.Unavailable},
		}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestServiceConfigJSONInvalidRejected(t *testing.T) {
	t.Parallel()

	cfg := &config{serviceConfig: `{"loadBalancingConfig":`, serviceConfigSet: true}

	_, err := cfg.serviceConfigJSON()
	if err == nil {
		t.Fatal("serviceConfigJSON succeeded for invalid JSON, want error")
	}
	if !strings.HasPrefix(err.Error(), "grpcclient:") {
		t.Fatalf("error = %q, want grpcclient: prefix", err.Error())
	}
}
