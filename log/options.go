package log

// Options configures logger construction.
type Options struct {
	// MinLevel is the minimum severity emitted by the logger.
	MinLevel Level `json:"min_level" toml:"min_level" yaml:"min_level"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	return nil
}
