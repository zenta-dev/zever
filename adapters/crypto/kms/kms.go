package kms

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
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/zenta-dev/zever/core/crypto"
)

// envelopeVersion is the single envelope format version this adapter reads and writes.
const envelopeVersion byte = 0x01

// nonceSize is the GCM nonce size for envelope data keys.
const nonceSize = 12

// randRead is a seam for testing nonce generation failure.
var randRead = rand.Read

// Client is the pluggable KMS backend. GenerateDataKey mints a data key
// under keyID; Decrypt recovers the plaintext data key from its encrypted form.
type Client interface {
	// GenerateDataKey returns plaintext and encrypted data key material for keyID.
	GenerateDataKey(ctx context.Context, keyID string) (plaintext, encrypted []byte, err error)
	// Decrypt returns the plaintext data key for encrypted material.
	Decrypt(ctx context.Context, encrypted []byte) (plaintext []byte, err error)
}

// stubClient is a deterministic dev/test KMS with no network. The data key
// is SHA256("zever-kms/v1:"+keyID) and the encrypted form is a tagged
// keyID string. Test-only: it provides no real KMS security boundary.
type stubClient struct{}

// StubClient returns the deterministic dev/test KMS client.
func StubClient() Client {
	return stubClient{}
}

func stubDataKey(keyID string) []byte {
	sum := sha256.Sum256([]byte("zever-kms/v1:" + keyID))
	return sum[:]
}

func stubEncrypted(keyID string) []byte {
	return []byte("stub-enc:" + keyID)
}

// GenerateDataKey mints a deterministic test data key for keyID.
func (stubClient) GenerateDataKey(_ context.Context, keyID string) ([]byte, []byte, error) {
	if strings.TrimSpace(keyID) == "" {
		return nil, nil, fmt.Errorf("kms stub: %w: empty key id", crypto.ErrInvalidKey)
	}

	plain := stubDataKey(keyID)

	return plain, stubEncrypted(keyID), nil
}

// Decrypt recovers the deterministic test data key.
func (stubClient) Decrypt(_ context.Context, encrypted []byte) ([]byte, error) {
	s := string(encrypted)
	if !strings.HasPrefix(s, "stub-enc:") {
		return nil, fmt.Errorf("kms stub: %w", crypto.ErrIntegrity)
	}

	keyID := strings.TrimPrefix(s, "stub-enc:")
	if strings.TrimSpace(keyID) == "" {
		return nil, fmt.Errorf("kms stub: %w", crypto.ErrIntegrity)
	}

	return stubDataKey(keyID), nil
}

// driver implements crypto.Crypto via KMS envelope AES-256-GCM.
type driver struct {
	keyID    string
	allowed  map[string]struct{}
	client   Client
	macKey   []byte
	hasMac   bool
	signPriv ed25519.PrivateKey
}

// New creates a crypto.Crypto using KMS envelope encryption with the
// deterministic stub client. Test and dev only: production must inject a
// real KMS client via NewWithClient.
func New(opts Options) (crypto.Crypto, error) {
	return NewWithClient(opts, stubClient{})
}

// NewWithClient creates a crypto.Crypto using the given KMS client.
func NewWithClient(opts Options, client Client) (crypto.Crypto, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("kms: %w", errors.Join(err, crypto.ErrInvalidKey))
	}

	if client == nil {
		return nil, fmt.Errorf("kms: %w: nil client", crypto.ErrInvalidKey)
	}

	var signPriv ed25519.PrivateKey
	if opts.SignKey != "" {
		signBytes, _ := base64.StdEncoding.DecodeString(opts.SignKey)
		signPriv = ed25519.PrivateKey(signBytes)
	}

	var macKey []byte
	hasMac := false
	if opts.Key != "" {
		key, _ := base64.StdEncoding.DecodeString(opts.Key)
		macKey, _ = hkdf.Key(sha256.New, key, nil, "zever/kms/mac", 32)
		hasMac = true
	}

	allowed := make(map[string]struct{}, len(opts.KeyIDs)+1)
	allowed[opts.KeyID] = struct{}{}
	for _, id := range opts.KeyIDs {
		allowed[id] = struct{}{}
	}

	return &driver{keyID: opts.KeyID, allowed: allowed, client: client, macKey: macKey, hasMac: hasMac, signPriv: signPriv}, nil
}

// Encrypt seals plaintext under a fresh KMS data key, returning
// version||keyID||encryptedDEK||nonce||ciphertext. Rotating KeyID/KeyIDs
// keeps old envelopes decryptable: Decrypt accepts any allowed key ID.
func (d *driver) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	plainDEK, encDEK, err := d.client.GenerateDataKey(ctx, d.keyID)
	if err != nil {
		return nil, fmt.Errorf("kms: generate data key: %w", err)
	}

	if len(plainDEK) != 32 {
		return nil, fmt.Errorf("kms: %w: data key must be 32 bytes", crypto.ErrInvalidKey)
	}

	block, err := aes.NewCipher(plainDEK)
	if err != nil {
		return nil, fmt.Errorf("kms: %w", crypto.ErrInvalidKey)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("kms: %w", crypto.ErrInvalidKey)
	}

	nonce := make([]byte, nonceSize)
	if _, err := randRead(nonce); err != nil {
		return nil, err
	}

	sealed := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, 1+2+len(d.keyID)+2+len(encDEK)+nonceSize+len(sealed))
	out = append(out, envelopeVersion)

	var lb [2]byte
	if len(d.keyID) > math.MaxUint16 {
		return nil, fmt.Errorf("kms: %w: key id too long", crypto.ErrInvalidKey)
	}
	binary.BigEndian.PutUint16(lb[:], uint16(len(d.keyID))) //nolint:gosec // length bounded above
	out = append(out, lb[:]...)
	out = append(out, d.keyID...)

	if len(encDEK) > math.MaxUint16 {
		return nil, fmt.Errorf("kms: %w: encrypted data key too long", crypto.ErrInvalidKey)
	}
	binary.BigEndian.PutUint16(lb[:], uint16(len(encDEK))) //nolint:gosec // length bounded above
	out = append(out, lb[:]...)
	out = append(out, encDEK...)
	out = append(out, nonce...)
	out = append(out, sealed...)

	return out, nil
}

// Decrypt opens an envelope sealed by Encrypt. Unknown versions, key IDs
// outside the allowed rotation set, KMS failures, and GCM failures all fail
// closed with crypto.ErrIntegrity and never echo key material.
func (d *driver) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 1 || ciphertext[0] != envelopeVersion {
		return nil, crypto.ErrIntegrity
	}

	rest := ciphertext[1:]
	if len(rest) < 2 {
		return nil, crypto.ErrIntegrity
	}

	keyLen := int(binary.BigEndian.Uint16(rest[:2]))
	rest = rest[2:]
	if len(rest) < keyLen+2 {
		return nil, crypto.ErrIntegrity
	}

	if _, ok := d.allowed[string(rest[:keyLen])]; !ok {
		return nil, crypto.ErrIntegrity
	}
	rest = rest[keyLen:]
	encLen := int(binary.BigEndian.Uint16(rest[:2]))
	rest = rest[2:]
	if len(rest) < encLen+nonceSize {
		return nil, crypto.ErrIntegrity
	}

	encDEK := rest[:encLen]
	rest = rest[encLen:]
	nonce := rest[:nonceSize]
	sealed := rest[nonceSize:]

	plainDEK, err := d.client.Decrypt(ctx, encDEK)
	if err != nil {
		return nil, crypto.ErrIntegrity
	}

	if len(plainDEK) != 32 {
		return nil, crypto.ErrIntegrity
	}

	block, err := aes.NewCipher(plainDEK)
	if err != nil {
		return nil, crypto.ErrIntegrity
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, crypto.ErrIntegrity
	}

	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, crypto.ErrIntegrity
	}

	return plaintext, nil
}

// Sign returns the Ed25519 signature for message. It requires SignKey.
func (d *driver) Sign(_ context.Context, message []byte) ([]byte, error) {
	if d.signPriv == nil {
		return nil, crypto.ErrKeyNotFound
	}

	return ed25519.Sign(d.signPriv, message), nil
}

// Verify reports whether signature is valid for message. It requires SignKey.
func (d *driver) Verify(_ context.Context, message, signature []byte) (bool, error) {
	if d.signPriv == nil {
		return false, crypto.ErrKeyNotFound
	}

	pub := d.signPriv.Public().(ed25519.PublicKey) //nolint:forcetypeassert // ed25519 PrivateKey always returns ed25519.PublicKey
	return ed25519.Verify(pub, message, signature), nil
}

// Mac returns the HMAC-SHA256 for message. It requires Key.
func (d *driver) Mac(_ context.Context, message []byte) ([]byte, error) {
	if !d.hasMac {
		return nil, crypto.ErrKeyNotFound
	}

	h := hmac.New(sha256.New, d.macKey)
	_, _ = h.Write(message)

	return h.Sum(nil), nil
}

// VerifyMac reports whether mac is valid for message. It requires Key.
func (d *driver) VerifyMac(_ context.Context, message, mac []byte) (bool, error) {
	if !d.hasMac {
		return false, crypto.ErrKeyNotFound
	}

	h := hmac.New(sha256.New, d.macKey)
	_, _ = h.Write(message)
	computed := h.Sum(nil)
	if !hmac.Equal(computed, mac) {
		return false, crypto.ErrIntegrity
	}

	return true, nil
}
