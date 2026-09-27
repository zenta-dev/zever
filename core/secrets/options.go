package secrets

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

// Validate checks options for consistency, joining all violations.
// There are currently no checks that hold universally across all adapters,
// so this is a no-op; each adapter's factory validates its own fields.
func (o Options) Validate() error {
	return nil
}
