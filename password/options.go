package password

// Default argon2id parameters with units noted per constant.
const (
	// DefaultTime is the default Argon2id iteration count.
	DefaultTime uint32 = 3
	// DefaultMemory is the default Argon2id memory cost in KiB (65536 KiB equals 64 MiB).
	DefaultMemory uint32 = 64 * 1024 // 64 MiB
	// DefaultThreads is the default Argon2id parallelism degree.
	DefaultThreads uint8 = 4
	// DefaultSaltLen is the default salt length in bytes.
	DefaultSaltLen uint32 = 16
	// DefaultKeyLen is the default derived key length in bytes.
	DefaultKeyLen uint32 = 32
)

// Options holds typed configuration for password hashing.
type Options struct {
	// Time is the Argon2id iteration count in the range 1-10.
	Time uint32 `json:"time"        toml:"time"        yaml:"time"`
	// Memory is the Argon2id memory cost in KiB in the range 8192-1048576.
	Memory uint32 `json:"memory"      toml:"memory"      yaml:"memory"`
	// Threads is the Argon2id parallelism degree in the range 1-16.
	Threads uint8 `json:"threads"     toml:"threads"     yaml:"threads"`
	// SaltLen is the salt length in bytes in the range 8-64.
	SaltLen uint32 `json:"salt_length" toml:"salt_length" yaml:"salt_length"`
	// KeyLen is the derived key length in bytes in the range 16-128.
	KeyLen uint32 `json:"key_length"  toml:"key_length"  yaml:"key_length"`
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
