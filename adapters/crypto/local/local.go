package local

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/core/crypto"
)

type localCrypto struct {
	aesKey   []byte
	gcm      cipher.AEAD
	macKey   []byte
	signPriv ed25519.PrivateKey
}

// randRead is a seam for testing nonce generation failure.
var randRead = rand.Read

// New creates a crypto.Crypto using local AES-256-GCM and HKDF-SHA256.
func New(opts crypto.Options) (crypto.Crypto, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("local: %w", errors.Join(err, crypto.ErrInvalidKey))
	}

	// Validate guarantees base64 success and correct lengths.
	key, _ := base64.StdEncoding.DecodeString(opts.Key)

	var signPriv ed25519.PrivateKey
	if opts.SignKey != "" {
		signBytes, _ := base64.StdEncoding.DecodeString(opts.SignKey)
		signPriv = ed25519.PrivateKey(signBytes)
	}

	macKey, _ := hkdf.Key(sha256.New, key, nil, "zever/local/mac", 32)

	block, _ := aes.NewCipher(key)

	gcm, _ := cipher.NewGCM(block)

	return &localCrypto{aesKey: key, gcm: gcm, macKey: macKey, signPriv: signPriv}, nil
}

// Encrypt seals plaintext with AES-256-GCM, returning nonce||ciphertext.
//
// No key-id/version is embedded in the output: rotating crypto.Options.Key
// makes every previously-encrypted value permanently undecryptable, with
// no built-in multi-key keyring or versioned envelope to fall back to
// (unlike password/argon2's PHC-format hashes, which embed their own
// algorithm/params and support NeedsRehash-driven migration). This is a
// deliberate scope decision, not an oversight: a rotation-capable envelope
// format is a real feature with no concrete second use case yet in this
// codebase (CLAUDE.md: no unnecessary abstractions until there's a
// concrete second use case). Callers that need key rotation for
// long-lived encrypted-at-rest data must build it themselves (e.g. decrypt
// with the old key, re-encrypt with the new one, migrate at rest) or wait
// for a future versioned-envelope addition to this package.
func (l *localCrypto) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, l.gcm.NonceSize())
	if _, err := randRead(nonce); err != nil {
		return nil, err
	}
	return l.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens a nonce||ciphertext value sealed by Encrypt with the
// current crypto.Options.Key. See Encrypt's doc comment for why a key
// rotation has no fallback path: ciphertext sealed under a since-rotated
// key can no longer be opened.
func (l *localCrypto) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < l.gcm.NonceSize() {
		return nil, crypto.ErrIntegrity
	}
	nonce := ciphertext[:l.gcm.NonceSize()]
	sealed := ciphertext[l.gcm.NonceSize():]
	plaintext, err := l.gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, crypto.ErrIntegrity
	}
	return plaintext, nil
}

func (l *localCrypto) Sign(_ context.Context, message []byte) ([]byte, error) {
	if l.signPriv == nil {
		return nil, crypto.ErrKeyNotFound
	}
	return ed25519.Sign(l.signPriv, message), nil
}

func (l *localCrypto) Verify(_ context.Context, message, signature []byte) (bool, error) {
	// Contract: (false, nil) means key present but signature invalid;
	// (false, ErrKeyNotFound) means no signing key configured. Callers
	// must check err before trusting ok=false as a plain mismatch.
	if l.signPriv == nil {
		return false, crypto.ErrKeyNotFound
	}
	pub := l.signPriv.Public().(ed25519.PublicKey) //nolint:forcetypeassert // ed25519 PrivateKey always returns ed25519.PublicKey
	return ed25519.Verify(pub, message, signature), nil
}

func (l *localCrypto) Mac(_ context.Context, message []byte) ([]byte, error) {
	h := hmac.New(sha256.New, l.macKey)
	_, _ = h.Write(message)
	return h.Sum(nil), nil
}

func (l *localCrypto) VerifyMac(_ context.Context, message, mac []byte) (bool, error) {
	// Contract differs from Verify by history: mismatch returns
	// (false, ErrIntegrity), not (false, nil). Callers must use
	// errors.Is(err, crypto.ErrIntegrity) to detect forgery; ok alone
	// is insufficient. Kept as-is to avoid breaking verifiers.
	h := hmac.New(sha256.New, l.macKey)
	_, _ = h.Write(message)
	computed := h.Sum(nil)
	if !hmac.Equal(computed, mac) {
		return false, crypto.ErrIntegrity
	}
	return true, nil
}
