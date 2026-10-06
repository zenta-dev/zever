package grpcclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
)

// defaultServiceConfig is the base service config applied to every
// connection: client-side round_robin load balancing.
const defaultServiceConfig = `{"loadBalancingConfig":[{"round_robin":{}}]}`

// RetryPolicy configures client-side retry merged into the gRPC service
// config for every method. The zero value is invalid; see validate.
type RetryPolicy struct {
	// MaxAttempts is the maximum number of RPC attempts including the
	// original. gRPC requires at least 2; values above DefaultMaxAttempts
	// are capped.
	MaxAttempts int
	// InitialBackoff is the base backoff between attempts. Must be
	// positive.
	InitialBackoff time.Duration
	// MaxBackoff caps the exponential backoff. Must be positive.
	MaxBackoff time.Duration
	// BackoffMultiplier scales the backoff after each attempt. Must be
	// positive.
	BackoffMultiplier float64
	// RetryableStatusCodes lists the status codes that trigger a retry.
	// Must not be empty.
	RetryableStatusCodes []codes.Code
}

// validate rejects policies gRPC would silently ignore: maxAttempts below
// 2, non-positive backoffs or multiplier, and empty status codes.
func (p RetryPolicy) validate() error {
	if p.MaxAttempts < 2 {
		return fmt.Errorf("grpcclient: retry policy: maxAttempts must be at least 2, got %d", p.MaxAttempts)
	}
	if p.InitialBackoff <= 0 {
		return fmt.Errorf("grpcclient: retry policy: initialBackoff must be positive, got %v", p.InitialBackoff)
	}
	if p.MaxBackoff <= 0 {
		return fmt.Errorf("grpcclient: retry policy: maxBackoff must be positive, got %v", p.MaxBackoff)
	}
	if p.BackoffMultiplier <= 0 {
		return fmt.Errorf("grpcclient: retry policy: backoffMultiplier must be positive, got %v", p.BackoffMultiplier)
	}
	if len(p.RetryableStatusCodes) == 0 {
		return errors.New("grpcclient: retry policy: retryableStatusCodes must not be empty")
	}
	return nil
}

// jsonRetryPolicy renders the policy as a gRPC service config retryPolicy
// object, validating first and capping MaxAttempts at DefaultMaxAttempts.
func (p RetryPolicy) jsonRetryPolicy() (map[string]any, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	maxAttempts := p.MaxAttempts
	if maxAttempts > DefaultMaxAttempts {
		maxAttempts = DefaultMaxAttempts
	}

	statusCodes := make([]string, 0, len(p.RetryableStatusCodes))
	for _, c := range p.RetryableStatusCodes {
		statusCodes = append(statusCodes, retryableCodeName(c))
	}

	return map[string]any{
		"maxAttempts":          maxAttempts,
		"initialBackoff":       grpcDuration(p.InitialBackoff),
		"maxBackoff":           grpcDuration(p.MaxBackoff),
		"backoffMultiplier":    p.BackoffMultiplier,
		"retryableStatusCodes": statusCodes,
	}, nil
}

// retryableCodeName returns the canonical gRPC status code name accepted by
// the service config parser ("UNAVAILABLE"). codes.Code.String is mixed case
// ("Unavailable") and is rejected by the parser.
func retryableCodeName(c codes.Code) string {
	switch c {
	case codes.OK:
		return "OK"
	case codes.Canceled:
		return "CANCELLED"
	case codes.Unknown:
		return "UNKNOWN"
	case codes.InvalidArgument:
		return "INVALID_ARGUMENT"
	case codes.DeadlineExceeded:
		return "DEADLINE_EXCEEDED"
	case codes.NotFound:
		return "NOT_FOUND"
	case codes.AlreadyExists:
		return "ALREADY_EXISTS"
	case codes.PermissionDenied:
		return "PERMISSION_DENIED"
	case codes.ResourceExhausted:
		return "RESOURCE_EXHAUSTED"
	case codes.FailedPrecondition:
		return "FAILED_PRECONDITION"
	case codes.Aborted:
		return "ABORTED"
	case codes.OutOfRange:
		return "OUT_OF_RANGE"
	case codes.Unimplemented:
		return "UNIMPLEMENTED"
	case codes.Internal:
		return "INTERNAL"
	case codes.Unavailable:
		return "UNAVAILABLE"
	case codes.DataLoss:
		return "DATA_LOSS"
	case codes.Unauthenticated:
		return "UNAUTHENTICATED"
	default:
		return "CODE(" + strconv.FormatInt(int64(c), 10) + ")"
	}
}

// grpcDuration formats d as a protobuf-JSON duration string ("0.01s"), the
// only format gRPC's service config parser accepts; time.Duration.String
// emits "10ms", which the parser rejects.
func grpcDuration(d time.Duration) string {
	sec := d / time.Second
	ns := d % time.Second

	str := fmt.Sprintf("%d.%09d", sec, ns)
	str = strings.TrimSuffix(str, "000")
	str = strings.TrimSuffix(str, "000")
	str = strings.TrimSuffix(str, ".000")

	return str + "s"
}

// serviceConfigJSON returns the service config JSON for the connection:
// the round_robin default, plus a per-method retryPolicy when a retry
// policy is configured, overlaid by any WithServiceConfigJSON keys.
func (c *config) serviceConfigJSON() (string, error) {
	base := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(defaultServiceConfig), &base); err != nil {
		return "", fmt.Errorf("grpcclient: default service config: %w", err)
	}

	if c.retry != nil {
		retryPolicy, err := c.retry.jsonRetryPolicy()
		if err != nil {
			return "", err
		}
		methodConfig, err := json.Marshal(map[string]any{
			"name":        []map[string]any{{}},
			"retryPolicy": retryPolicy,
		})
		if err != nil {
			return "", fmt.Errorf("grpcclient: retry policy: %w", err)
		}
		methodConfigSlice, err := json.Marshal([]json.RawMessage{methodConfig})
		if err != nil {
			return "", fmt.Errorf("grpcclient: retry policy: %w", err)
		}
		base["methodConfig"] = methodConfigSlice
	}

	if c.serviceConfigSet {
		overlay := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(c.serviceConfig), &overlay); err != nil {
			return "", fmt.Errorf("grpcclient: service config JSON: %w", err)
		}
		for k, v := range overlay {
			base[k] = v
		}
	}

	out, err := json.Marshal(base)
	if err != nil {
		return "", fmt.Errorf("grpcclient: service config: %w", err)
	}

	return string(out), nil
}
