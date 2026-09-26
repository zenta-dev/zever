package storage

// PolicyStore holds the resolved per-bucket policies and default policy.
type PolicyStore struct {
	policies   map[BucketName]Policy
	defaultPol Policy
	configured bool
}

// ResolveFromConfig resolves cfg into the store, replacing prior state.
func (s *PolicyStore) ResolveFromConfig(cfg *PolicyConfig) error {
	policies, def, configured, err := cfg.Resolve()
	if err != nil {
		return err
	}

	s.policies = policies
	s.defaultPol = def
	s.configured = configured

	return nil
}

// PolicyFor returns the policy for bucket, or the default when unlisted.
func (s *PolicyStore) PolicyFor(bucket string) Policy {
	if p, ok := s.policies[BucketName(bucket)]; ok {
		return p
	}

	return s.defaultPol
}

// Configured reports whether any policy was configured.
func (s *PolicyStore) Configured() bool {
	return s.configured
}

// Policies returns the resolved per-bucket policies.
func (s *PolicyStore) Policies() map[BucketName]Policy {
	return s.policies
}

// Default returns the fallback policy for unlisted buckets.
func (s *PolicyStore) Default() Policy {
	return s.defaultPol
}
