package argon2

import (
	"github.com/zenta-dev/zever/core/password"
)

// Default parameters tuned for ~200ms on modern hardware per OWASP 2023.
// Memory is in KiB.
const (
	DefaultTime    uint32 = 3
	DefaultMemory  uint32 = 64 * 1024 // 64 MiB
	DefaultThreads uint8  = 4
	DefaultSaltLen uint32 = 16
	DefaultKeyLen  uint32 = 32
)

// Options holds typed configuration for the argon2id hasher.
type Options struct {
	// Time is the number of iterations.
	Time uint32 `json:"time" toml:"time" yaml:"time"`
	// Memory is the memory usage in KiB.
	Memory uint32 `json:"memory" toml:"memory" yaml:"memory"`
	// Threads is the parallelism degree.
	Threads uint8 `json:"threads" toml:"threads" yaml:"threads"`
	// SaltLen is the salt length in bytes.
	SaltLen uint32 `json:"salt_length" toml:"salt_length" yaml:"salt_length"`
	// KeyLen is the derived key length in bytes.
	KeyLen uint32 `json:"key_length" toml:"key_length" yaml:"key_length"`
}

// ParseOptionsTyped fills zero values with defaults, then validates.
func ParseOptionsTyped(po password.Options) (Options, error) {
	o := Options{
		Time:    po.Time,
		Memory:  po.Memory,
		Threads: po.Threads,
		SaltLen: po.SaltLen,
		KeyLen:  po.KeyLen,
	}

	if o.Time == 0 {
		o.Time = DefaultTime
	}

	if o.Memory == 0 {
		o.Memory = DefaultMemory
	}

	if o.Threads == 0 {
		o.Threads = DefaultThreads
	}

	if o.SaltLen == 0 {
		o.SaltLen = DefaultSaltLen
	}

	if o.KeyLen == 0 {
		o.KeyLen = DefaultKeyLen
	}

	if err := o.Validate(); err != nil {
		return Options{}, err
	}

	return o, nil
}

// Validate checks parameter ranges.
func (o Options) Validate() error {
	if o.Time < 1 || o.Time > 10 {
		return password.ErrInvalidHash
	}

	if o.Memory < 8*1024 || o.Memory > 1024*1024 {
		return password.ErrInvalidHash
	}

	if o.Threads < 1 || o.Threads > 16 {
		return password.ErrInvalidHash
	}

	if o.SaltLen < 8 || o.SaltLen > 64 {
		return password.ErrInvalidHash
	}

	if o.KeyLen < 16 || o.KeyLen > 128 {
		return password.ErrInvalidHash
	}

	return nil
}
