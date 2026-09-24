package observability

import (
	"crypto/x509"
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// Options configures provider construction.
type Options struct {
	// Endpoint is the collector address as host:port.
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// ServiceName identifies the service emitting telemetry.
	ServiceName string `json:"service_name" toml:"service_name" yaml:"service_name"`
	// Insecure disables TLS; only allowed for loopback endpoints.
	Insecure bool `json:"insecure" toml:"insecure" yaml:"insecure"`
	// SetGlobals registers the provider as the global default.
	SetGlobals bool `json:"set_globals" toml:"set_globals" yaml:"set_globals"`
	// Verbose enables debug logging.
	Verbose bool `json:"verbose" toml:"verbose" yaml:"verbose"`
	// CAFile is the path to a PEM-encoded CA bundle.
	CAFile string `json:"ca_file" toml:"ca_file" yaml:"ca_file"`
	// CertFile is the path to a PEM-encoded client certificate.
	CertFile string `json:"cert_file" toml:"cert_file" yaml:"cert_file"`
	// KeyFile is the path to a PEM-encoded client key.
	KeyFile string `json:"key_file" toml:"key_file" yaml:"key_file"`
	// Headers are outbound request headers.
	Headers map[string]string `json:"headers" toml:"headers" yaml:"headers"`
	// SampleRatio is the trace sampling probability in [0,1].
	SampleRatio float64 `json:"sample_ratio" toml:"sample_ratio" yaml:"sample_ratio"`
	// AttrValueLimit caps string attribute values; 0 selects the default.
	AttrValueLimit int `json:"attr_value_limit" toml:"attr_value_limit" yaml:"attr_value_limit"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	if len(o.ServiceName) < 1 || len(o.ServiceName) > 128 {
		return &InvalidOptionsError{Reason: "service_name must be 1-128 characters"}
	}
	for _, r := range o.ServiceName {
		if unicode.IsControl(r) {
			return &InvalidOptionsError{Reason: "service_name must not contain control characters"}
		}
	}

	if o.Endpoint != "" {
		if err := validateEndpoint(o.Endpoint); err != nil {
			return err
		}
	}

	if o.Insecure && o.Endpoint != "" {
		host, _, _ := net.SplitHostPort(o.Endpoint)
		if !isLoopback(host) {
			return &InvalidOptionsError{Reason: "insecure requires a loopback endpoint"}
		}
	}

	if o.CAFile != "" {
		if o.Insecure {
			return &InvalidOptionsError{Reason: "ca file requires secure mode"}
		}
		pemData, err := os.ReadFile(o.CAFile)
		if err != nil {
			return &InvalidOptionsError{Reason: fmt.Sprintf("unreadable CA file %q", o.CAFile)}
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			return &InvalidOptionsError{Reason: "invalid ca file contents"}
		}
	}

	if (o.CertFile == "") != (o.KeyFile == "") {
		return &InvalidOptionsError{Reason: "cert_file and key_file must be set together"}
	}

	if len(o.Headers) > 32 {
		return &InvalidOptionsError{Reason: "too many headers"}
	}
	for k := range o.Headers {
		if !isToken(k) {
			return &InvalidOptionsError{Reason: fmt.Sprintf("invalid header key %q", k)}
		}
		if o.Insecure && strings.EqualFold(k, "authorization") {
			return &InvalidOptionsError{Reason: "authorization_header requires secure mode"}
		}
	}

	if math.IsNaN(o.SampleRatio) || o.SampleRatio < 0 || o.SampleRatio > 1 {
		return &InvalidOptionsError{Reason: "sample_ratio must be in [0,1]"}
	}

	if o.AttrValueLimit != 0 && (o.AttrValueLimit < 256 || o.AttrValueLimit > 16384) {
		return &InvalidOptionsError{Reason: "attribute_value_limit must be 0 or in [256,16384]"}
	}

	return nil
}

func validateEndpoint(endpoint string) error {
	if strings.Contains(endpoint, "://") {
		return &InvalidOptionsError{Reason: fmt.Sprintf("invalid endpoint %q", endpoint)}
	}
	if strings.ContainsAny(endpoint, "/?#") {
		return &InvalidOptionsError{Reason: fmt.Sprintf("invalid endpoint %q", endpoint)}
	}
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		return &InvalidOptionsError{Reason: fmt.Sprintf("invalid endpoint %q", endpoint)}
	}
	if host == "" {
		return &InvalidOptionsError{Reason: fmt.Sprintf("invalid endpoint %q", endpoint)}
	}
	if host == "0.0.0.0" || host == "::" {
		return &InvalidOptionsError{Reason: fmt.Sprintf("unspecified endpoint host %q", endpoint)}
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return &InvalidOptionsError{Reason: fmt.Sprintf("invalid endpoint port %q", endpoint)}
	}
	return nil
}

func isLoopback(host string) bool {
	lowered := strings.ToLower(strings.TrimSpace(host))
	if lowered == "localhost" {
		return true
	}
	if strings.HasPrefix(lowered, "127.") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func isToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9':
			continue
		case strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)):
			continue
		default:
			return false
		}
	}
	return true
}
