package secrets

import "fmt"

// Adapter identifies the secrets backend implementation.
type Adapter int

const (
	// Env is the environment-variable secrets adapter.
	Env Adapter = iota
	// Vault is the HashiCorp Vault secrets adapter.
	Vault
	// GCP is the Google Cloud Secret Manager adapter.
	GCP
	// AWS is the AWS Secrets Manager adapter.
	AWS
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Env:
		return "env"
	case Vault:
		return "vault"
	case GCP:
		return "gcp"
	case AWS:
		return "aws"
	default:
		return fmt.Sprintf("Adapter(%d)", int(a))
	}
}

// ParseAdapter parses adapter name into an Adapter.
// It returns InvalidAdapterError for unknown names.
func ParseAdapter(adapter string) (Adapter, error) {
	switch adapter {
	case "env":
		return Env, nil
	case "vault":
		return Vault, nil
	case "gcp":
		return GCP, nil
	case "aws":
		return AWS, nil
	default:
		return Env, &InvalidAdapterError{Adapter: adapter}
	}
}
