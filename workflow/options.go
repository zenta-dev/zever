package workflow

import (
	"errors"
	"fmt"
	"net"
)

// Options holds typed configuration for workflow adapters.
type Options struct {
	// HostPort is the workflow server address.
	HostPort string `json:"host_port" toml:"host_port" yaml:"host_port"`
	// Namespace is the workflow namespace.
	Namespace string `json:"namespace" toml:"namespace" yaml:"namespace"`
}

// Validate checks Options for adapter-independent errors.
func (o Options) Validate() error {
	var errs []error

	if o.HostPort != "" {
		if _, _, err := net.SplitHostPort(o.HostPort); err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: fmt.Sprintf("invalid host_port %q", o.HostPort)})
		}
	}

	return errors.Join(errs...)
}
