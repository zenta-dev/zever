package job

// Definition bundles a registered job's handler, retry policy, and priority.
type Definition struct {
	handler  Handler
	policy   RetryPolicy
	priority Priority
}

// Option configures a Definition during registration.
type Option func(*Definition)

// WithRetryPolicy returns an Option that sets the job retry policy.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(d *Definition) { d.policy = p }
}

// WithPriority returns an Option that sets the job priority.
func WithPriority(p Priority) Option {
	return func(d *Definition) { d.priority = p }
}
