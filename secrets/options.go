package secrets

import "fmt"

// Options holds typed configuration for secrets adapters.
// Fields are a union of all adapter options; each adapter uses only what it needs.
type Options struct {
	// Addr is the Vault server address.
	Addr string `json:"addr" toml:"addr" yaml:"addr"`
	// Token is the Vault authentication token.
	Token string `json:"token" toml:"token" yaml:"token"`
	// Mount is the Vault KV mount path.
	Mount string `json:"mount" toml:"mount" yaml:"mount"`
	// ProjectID is the GCP project ID.
	ProjectID string `json:"project_id" toml:"project_id" yaml:"project_id"`
	// Prefix scopes secret names to one namespace.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
	// Region is the AWS region.
	Region string `json:"region" toml:"region" yaml:"region"`
}

// ParseOptions extracts a typed Options from the raw option map.
// Unknown keys and non-string values return an error.
func ParseOptions(m map[string]any) (Options, error) {
	var o Options
	if m == nil {
		return o, nil
	}

	for k, v := range m {
		switch k {
		case "addr":
			s, ok := v.(string)
			if !ok {
				return o, fmt.Errorf("secrets: option %q must be a string, got %T", k, v)
			}

			o.Addr = s
		case "token":
			s, ok := v.(string)
			if !ok {
				return o, fmt.Errorf("secrets: option %q must be a string, got %T", k, v)
			}

			o.Token = s
		case "mount":
			s, ok := v.(string)
			if !ok {
				return o, fmt.Errorf("secrets: option %q must be a string, got %T", k, v)
			}

			o.Mount = s
		case "project_id":
			s, ok := v.(string)
			if !ok {
				return o, fmt.Errorf("secrets: option %q must be a string, got %T", k, v)
			}

			o.ProjectID = s
		case "prefix":
			s, ok := v.(string)
			if !ok {
				return o, fmt.Errorf("secrets: option %q must be a string, got %T", k, v)
			}

			o.Prefix = s
		case "region":
			s, ok := v.(string)
			if !ok {
				return o, fmt.Errorf("secrets: option %q must be a string, got %T", k, v)
			}

			o.Region = s
		default:
			return o, fmt.Errorf("secrets: unknown option %q", k)
		}
	}

	return o, nil
}

// Validate performs battery-level checks that hold across every adapter.
// There are currently no checks that hold universally across all adapters,
// so this is a no-op; each adapter's factory validates its own fields.
func (o Options) Validate() error {
	return nil
}
