package argon2

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/zenta-dev/zever/password"
)

// maxPasswordLength guards derivation cost against oversized inputs.
const maxPasswordLength = 1024

// randRead is a seam for injecting salt-generation failures in tests.
var randRead = rand.Read

type hasher struct {
	opts Options
}

// New creates a password.Hasher from typed options, filling defaults.
func New(opts password.Options) (password.Hasher, error) {
	parsed, err := ParseOptionsTyped(opts)
	if err != nil {
		return nil, err
	}

	return &hasher{opts: parsed}, nil
}

func (h *hasher) Hash(_ context.Context, pwd string) (string, error) {
	if len(pwd) > maxPasswordLength {
		return "", password.ErrPasswordTooLong
	}

	salt := make([]byte, h.opts.SaltLen)
	if _, err := randRead(salt); err != nil {
		return "", fmt.Errorf("argon2id: generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(pwd), salt, h.opts.Time, h.opts.Memory, h.opts.Threads, h.opts.KeyLen)

	return encodeHash(h.opts.Time, h.opts.Memory, h.opts.Threads, salt, hash), nil
}

func (h *hasher) Verify(_ context.Context, hash, pwd string) (bool, error) {
	if len(pwd) > maxPasswordLength {
		return false, password.ErrPasswordTooLong
	}

	params, salt, expected, err := decodeHash(hash)
	if err != nil {
		return false, err
	}

	computed := argon2.IDKey([]byte(pwd), salt, params.time, params.memory, params.threads, uint32(len(expected))) //nolint:gosec // G115: len comes from a successfully decoded stored hash, not attacker input

	if subtle.ConstantTimeCompare(computed, expected) == 1 {
		return true, nil
	}

	return false, nil
}

func (h *hasher) NeedsRehash(_ context.Context, hash string) (bool, error) {
	params, _, expected, err := decodeHash(hash)
	if err != nil {
		return false, err
	}

	if params.time != h.opts.Time || params.memory != h.opts.Memory || params.threads != h.opts.Threads {
		return true, nil
	}

	if uint32(len(expected)) != h.opts.KeyLen { //nolint:gosec // G115: len comes from a successfully decoded stored hash, not attacker input
		return true, nil
	}

	return false, nil
}

// encodeHash formats as PHC: $argon2id$v=19$m=MEMORY,t=TIME,p=THREADS$SALT$HASH.
func encodeHash(time, memory uint32, threads uint8, salt, hash []byte) string {
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, time, threads, b64Salt, b64Hash)
}

type decodedParams struct {
	time    uint32
	memory  uint32
	threads uint8
	hash    []byte
}

func decodeHash(encoded string) (decodedParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// Expected: ["", "argon2id", "v=19", "m=...,t=...,p=...", "salt", "hash"]
	if len(parts) != 6 {
		return decodedParams{}, nil, nil, password.ErrInvalidHash
	}

	if parts[1] != "argon2id" {
		return decodedParams{}, nil, nil, password.ErrInvalidHash
	}

	if parts[2] != "v=19" {
		return decodedParams{}, nil, nil, password.ErrInvalidHash
	}

	paramsStr := parts[3]
	saltB64 := parts[4]
	hashB64 := parts[5]

	if paramsStr == "" || saltB64 == "" || hashB64 == "" {
		return decodedParams{}, nil, nil, password.ErrInvalidHash
	}

	var memory, time uint32

	var threads uint8

	foundM, foundT, foundP := false, false, false

	for _, kv := range strings.Split(paramsStr, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return decodedParams{}, nil, nil, password.ErrInvalidHash
		}

		switch k {
		case "m":
			n, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return decodedParams{}, nil, nil, password.ErrInvalidHash
			}

			memory = uint32(n)
			foundM = true
		case "t":
			n, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return decodedParams{}, nil, nil, password.ErrInvalidHash
			}

			time = uint32(n)
			foundT = true
		case "p":
			n, err := strconv.ParseUint(v, 10, 8)
			if err != nil {
				return decodedParams{}, nil, nil, password.ErrInvalidHash
			}

			threads = uint8(n)
			foundP = true
		default:
			return decodedParams{}, nil, nil, password.ErrInvalidHash
		}
	}

	if !foundM || !foundT || !foundP {
		return decodedParams{}, nil, nil, password.ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(saltB64)
	if err != nil {
		return decodedParams{}, nil, nil, password.ErrInvalidHash
	}

	hash, err := base64.RawStdEncoding.DecodeString(hashB64)
	if err != nil {
		return decodedParams{}, nil, nil, password.ErrInvalidHash
	}

	if len(salt) < 8 || len(hash) < 16 {
		return decodedParams{}, nil, nil, password.ErrInvalidHash
	}

	dp := decodedParams{time: time, memory: memory, threads: threads, hash: hash}

	return dp, salt, hash, nil
}
