package crypto

import (
	"context"
)

// Encryptor is the interface for encryption operations.
type Encryptor interface {
	// Encrypt seals plaintext and returns the ciphertext.
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	// Decrypt opens ciphertext and returns the plaintext.
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// Signer is the interface for signing operations.
type Signer interface {
	// Sign returns the signature for message.
	Sign(ctx context.Context, message []byte) ([]byte, error)
	// Verify reports whether signature is valid for message.
	Verify(ctx context.Context, message, signature []byte) (bool, error)
	// Mac returns the message authentication code for message.
	Mac(ctx context.Context, message []byte) ([]byte, error)
	// VerifyMac reports whether mac is valid for message.
	VerifyMac(ctx context.Context, message, mac []byte) (bool, error)
}

// Crypto is the interface that cryptographic adapters must implement.
type Crypto interface {
	// Encryptor seals and opens plaintext.
	Encryptor
	// Signer signs and authenticates messages.
	Signer
}

// Factory creates a Crypto from the given Options.
type Factory func(opts Options) (Crypto, error)
