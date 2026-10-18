package secrets

// Options holds typed configuration for secrets adapters.
// Fields are a union of all adapter options; each adapter uses only what it needs.
type Options struct {
	// Addr is the Vault server address.
	Addr string `json:"addr"`
	// Token is the Vault authentication token.
	Token string `json:"token" zever:"secret"`
	// TokenFile reads the Vault token from a file when Token is empty.
	TokenFile string `json:"token_file"`
	// Mount is the Vault KV mount path.
	Mount string `json:"mount"`
	// Namespace is the Vault namespace.
	Namespace string `json:"namespace"`
	// AllowInsecure permits a plain-http Vault Addr for test setups.
	// Default false (https only).
	AllowInsecure bool `json:"allow_insecure"`
	// AllowInsecureTokenFilePerms permits a Vault TokenFile readable
	// beyond its owner. Default false (0600 or stricter required).
	AllowInsecureTokenFilePerms bool `json:"allow_insecure_token_file_perms"`
	// ProjectID is the GCP project ID.
	ProjectID string `json:"project_id"`
	// Prefix scopes secret names to one namespace.
	Prefix string `json:"prefix"`
	// Region is the AWS region.
	Region string `json:"region"`
}

// Validate checks options for consistency, joining all violations.
// There are currently no checks that hold universally across all adapters,
// so this is a no-op; each adapter's factory validates its own fields.
func (o Options) Validate() error {
	return nil
}
