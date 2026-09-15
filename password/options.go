package password

// Default password-hashing parameters. Memory is in KiB.
const (
	DefaultTime    uint32 = 3
	DefaultMemory  uint32 = 64 * 1024 // 64 MiB
	DefaultThreads uint8  = 4
	DefaultSaltLen uint32 = 16
	DefaultKeyLen  uint32 = 32
)

// Options holds typed configuration for password hashing.
type Options struct {
	Time    uint32 `json:"time"        toml:"time"        yaml:"time"`
	Memory  uint32 `json:"memory"      toml:"memory"      yaml:"memory"`
	Threads uint8  `json:"threads"     toml:"threads"     yaml:"threads"`
	SaltLen uint32 `json:"salt_length" toml:"salt_length" yaml:"salt_length"`
	KeyLen  uint32 `json:"key_length"  toml:"key_length"  yaml:"key_length"`
}

// Validate checks parameter ranges.
//
// Each violation returns the bare ErrInvalidHash sentinel. This mirrors the
// parity source: Options is validated against a stored hash's parameters, so
// any out-of-range value means the hash itself cannot be trusted, not that a
// specific field was misconfigured. Callers needing field-level detail should
// inspect Options before hashing.
func (o Options) Validate() error {
	if o.Time < 1 || o.Time > 10 {
		return ErrInvalidHash
	}

	if o.Memory < 8*1024 || o.Memory > 1024*1024 {
		return ErrInvalidHash
	}

	if o.Threads < 1 || o.Threads > 16 {
		return ErrInvalidHash
	}

	if o.SaltLen < 8 || o.SaltLen > 64 {
		return ErrInvalidHash
	}

	if o.KeyLen < 16 || o.KeyLen > 128 {
		return ErrInvalidHash
	}

	return nil
}
