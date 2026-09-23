package log

// Options configures logger construction.
type Options struct {
	// MinLevel is the minimum severity emitted by the logger.
	MinLevel Level `json:"minlevel" toml:"minlevel" yaml:"minlevel"`
}
