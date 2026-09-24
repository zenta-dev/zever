package s3opts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestDefaultRegion(t *testing.T) {
	t.Parallel()
	if DefaultRegion != "us-east-1" {
		t.Fatalf("DefaultRegion = %q, want us-east-1", DefaultRegion)
	}
}

func TestWithDefaults(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		in            Config
		defaultRegion string
		wantRegion    string
	}{
		{"empty defaults to us-east-1", Config{}, "", "us-east-1"},
		{"empty with custom default", Config{}, "auto", "auto"},
		{"keeps explicit", Config{Region: "eu-west-1"}, "", "eu-west-1"},
		{"trims region", Config{Region: "  eu-west-1  "}, "", "eu-west-1"},
		{"trims fields", Config{Endpoint: "  https://example.com  ", Bucket: "  b  ", AccessKeyID: "  ak  ", SecretAccessKey: "  sk  "}, "", "us-east-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.in.WithDefaults(tc.defaultRegion)
			if got.Region != tc.wantRegion {
				t.Fatalf("Region = %q, want %q", got.Region, tc.wantRegion)
			}
		})
	}
	got := Config{Endpoint: "  https://example.com  ", Bucket: "  b  ", AccessKeyID: "  ak  ", SecretAccessKey: "  sk  "}.WithDefaults("")
	if got.Endpoint != "https://example.com" || got.Bucket != "b" || got.AccessKeyID != "ak" || got.SecretAccessKey != "sk" {
		t.Fatalf("WithDefaults did not trim fields: %+v", got)
	}
}

func TestValidateBucket(t *testing.T) {
	t.Parallel()
	if err := (Config{Bucket: "b"}).ValidateBucket(); err != nil {
		t.Fatalf("ValidateBucket valid = %v, want nil", err)
	}
	for _, tc := range []struct {
		name string
		in   Config
	}{
		{"empty", Config{}},
		{"whitespace", Config{Bucket: "   "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.in.ValidateBucket(); !errors.Is(err, ErrMissingBucket) {
				t.Fatalf("ValidateBucket err = %v, want ErrMissingBucket", err)
			}
		})
	}
}

func TestValidateCredentials(t *testing.T) {
	t.Parallel()
	if err := (Config{AccessKeyID: "ak", SecretAccessKey: "sk"}).ValidateCredentials(); err != nil {
		t.Fatalf("ValidateCredentials valid = %v, want nil", err)
	}
	cases := []struct {
		name string
		in   Config
	}{
		{"both empty", Config{}},
		{"key empty", Config{SecretAccessKey: "sk"}},
		{"secret empty", Config{AccessKeyID: "ak"}},
		{"key whitespace", Config{AccessKeyID: "   ", SecretAccessKey: "sk"}},
		{"secret whitespace", Config{AccessKeyID: "ak", SecretAccessKey: "  "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.in.ValidateCredentials()
			if !errors.Is(err, ErrMissingCredentials) {
				t.Fatalf("ValidateCredentials err = %v, want ErrMissingCredentials", err)
			}
			if strings.Contains(err.Error(), "sk") {
				t.Fatalf("error leaks secret: %q", err.Error())
			}
		})
	}
}

func TestValidateEndpoint(t *testing.T) {
	t.Parallel()
	valid := []string{"", "https://example.com", "https://example.com/x", "http://localhost:9000"}
	for _, e := range valid {
		if err := (Config{Endpoint: e}).ValidateEndpoint(); err != nil {
			t.Errorf("ValidateEndpoint(%q) = %v, want nil", e, err)
		}
	}
	cases := []struct {
		name     string
		endpoint string
		wantSub  string
	}{
		{"no scheme", "example.com/x", "scheme"},
		{"no host", "https:///path", "host"},
		{"garbage", "http://[::1", "valid URL"},
		{"ftp scheme", "ftp://invalid.example.com", "scheme"},
		{"userinfo", "https://user:pass@example.com", "user info"},
		{"whitespace", "https://example.com/a b", "whitespace"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := (Config{Endpoint: tc.endpoint}).ValidateEndpoint()
			if !errors.Is(err, ErrInvalidEndpoint) {
				t.Fatalf("ValidateEndpoint(%q) err = %v, want ErrInvalidEndpoint", tc.endpoint, err)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantSub)) {
				t.Fatalf("ValidateEndpoint(%q) err %q missing %q", tc.endpoint, err.Error(), tc.wantSub)
			}
		})
	}
}

func TestValidateRegion(t *testing.T) {
	t.Parallel()
	for _, r := range []string{"us-east-1", "auto", "eu-west-1"} {
		if err := (Config{Region: r}).ValidateRegion(); err != nil {
			t.Errorf("ValidateRegion(%q) = %v, want nil", r, err)
		}
	}
	for _, tc := range []struct {
		name string
		in   Config
	}{
		{"empty", Config{}},
		{"whitespace", Config{Region: "   "}},
		{"inner space", Config{Region: "us east 1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.in.ValidateRegion(); !errors.Is(err, ErrInvalidRegion) {
				t.Fatalf("ValidateRegion err = %v, want ErrInvalidRegion", err)
			}
		})
	}
}

func TestValidateJoined(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	err := cfg.Validate(true, true)
	if !errors.Is(err, ErrMissingBucket) {
		t.Fatalf("Validate err = %v, want ErrMissingBucket", err)
	}
	if !errors.Is(err, ErrMissingCredentials) {
		t.Fatalf("Validate err = %v, want ErrMissingCredentials", err)
	}
	if !errors.Is(err, ErrInvalidRegion) {
		t.Fatalf("Validate err = %v, want ErrInvalidRegion", err)
	}
	ok := Config{Endpoint: "https://example.com", Region: "us-east-1", Bucket: "b", AccessKeyID: "ak", SecretAccessKey: "sk"}
	if err := ok.Validate(true, true); err != nil {
		t.Fatalf("Validate valid = %v, want nil", err)
	}
	noBucket := Config{Endpoint: "", Region: "us-east-1", AccessKeyID: "ak", SecretAccessKey: "sk"}
	if err := noBucket.Validate(false, true); err != nil {
		t.Fatalf("Validate without bucket = %v, want nil", err)
	}
	noCreds := Config{Endpoint: "", Region: "us-east-1", Bucket: "b"}
	if err := noCreds.Validate(true, false); err != nil {
		t.Fatalf("Validate without creds = %v, want nil", err)
	}
	badEndpoint := Config{Endpoint: "ftp://x.example.com", Region: "us-east-1", Bucket: "b", AccessKeyID: "ak", SecretAccessKey: "sk"}
	if err := badEndpoint.Validate(true, true); !errors.Is(err, ErrInvalidEndpoint) {
		t.Fatalf("Validate bad endpoint = %v, want ErrInvalidEndpoint", err)
	}
}

func TestNoSecretLogging(t *testing.T) {
	t.Parallel()
	cfg := Config{Endpoint: "https://example.com", Region: "us-east-1", Bucket: "b", AccessKeyID: "mykey", SecretAccessKey: "supersecret123"}
	if err := cfg.ValidateCredentials(); err != nil {
		t.Fatal(err)
	}
	bad := Config{AccessKeyID: "", SecretAccessKey: ""}
	err := bad.Validate(true, true)
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "supersecret123") || strings.Contains(err.Error(), "mykey") {
		t.Fatalf("error leaks credentials: %q", err.Error())
	}
	good := cfg.WithDefaults("")
	if good.SecretAccessKey != "supersecret123" {
		t.Fatalf("WithDefaults altered secret")
	}
}

func TestNewClientSuccess(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cfg := Config{Region: "", AccessKeyID: "ak", SecretAccessKey: "sk"}.WithDefaults("")
	client, presigner, err := NewClient(ctx, "s3", cfg, "")
	if err != nil {
		t.Fatalf("NewClient = %v, want nil", err)
	}
	if client == nil || presigner == nil {
		t.Fatal("NewClient returned nil client/presigner")
	}
	if got := client.Options().Region; got != "us-east-1" {
		t.Fatalf("Region = %q, want us-east-1", got)
	}
}

func TestNewClientCustomEndpoint(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cfg := Config{Endpoint: "http://localhost:9000", Region: "us-east-1", AccessKeyID: "ak", SecretAccessKey: "sk"}
	client, _, err := NewClient(ctx, "s3", cfg, "")
	if err != nil {
		t.Fatalf("NewClient = %v, want nil", err)
	}
	ep := client.Options().BaseEndpoint
	if ep == nil || *ep != "http://localhost:9000" {
		t.Fatalf("BaseEndpoint = %v, want http://localhost:9000", ep)
	}
}

func TestNewClientURLBasePreferred(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cfg := Config{Endpoint: "http://localhost:9000", Region: "us-east-1", AccessKeyID: "ak", SecretAccessKey: "sk"}
	client, _, err := NewClient(ctx, "s3", cfg, "https://cdn.example.com")
	if err != nil {
		t.Fatalf("NewClient = %v, want nil", err)
	}
	ep := client.Options().BaseEndpoint
	if ep == nil || *ep != "https://cdn.example.com" {
		t.Fatalf("BaseEndpoint = %v, want urlBase", ep)
	}
}

func TestNewClientValidationErrors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cases := []struct {
		name    string
		cfg     Config
		urlBase string
		want    error
	}{
		{"bad endpoint", Config{Endpoint: "ftp://x.example.com", Region: "us-east-1"}, "", ErrInvalidEndpoint},
		{"bad urlbase", Config{Endpoint: "", Region: "us-east-1"}, "bogus", ErrInvalidEndpoint},
		{"bad region", Config{Endpoint: "", Region: "  "}, "", ErrInvalidRegion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := NewClient(ctx, "s3", tc.cfg, tc.urlBase)
			if !errors.Is(err, tc.want) {
				t.Fatalf("NewClient err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNewClientEmptyRegionDefaults(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cfg := Config{Endpoint: "", Region: "", AccessKeyID: "ak", SecretAccessKey: "sk"}
	client, _, err := NewClient(ctx, "s3", cfg, "")
	if err != nil {
		t.Fatalf("NewClient = %v, want nil", err)
	}
	if got := client.Options().Region; got != "us-east-1" {
		t.Fatalf("Region = %q, want us-east-1", got)
	}
}

func TestValidateURLEmpty(t *testing.T) {
	t.Parallel()
	if err := validateURL(""); err != nil {
		t.Fatalf("validateURL empty = %v, want nil", err)
	}
	if err := (Config{Endpoint: "   "}).ValidateEndpoint(); err != nil {
		t.Fatalf("ValidateEndpoint whitespace-only = %v, want nil", err)
	}
}

func TestNewClientCoreError(t *testing.T) {
	old := coreNewClient
	defer func() { coreNewClient = old }()
	want := errors.New("core boom")
	coreNewClient = func(context.Context, string, string, string, string, string, string) (*s3.Client, *s3.PresignClient, error) {
		return nil, nil, want
	}
	_, _, err := NewClient(t.Context(), "s3", Config{Region: "us-east-1"}, "")
	if !errors.Is(err, want) {
		t.Fatalf("NewClient err = %v, want core error", err)
	}
}

func TestNewClientNoSecretInError(t *testing.T) {
	t.Parallel()
	_, _, err := NewClient(t.Context(), "s3", Config{Endpoint: "ftp://x.example.com", Region: "us-east-1"}, "")
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "sk") {
		t.Fatalf("error leaks secret: %q", err.Error())
	}
}
