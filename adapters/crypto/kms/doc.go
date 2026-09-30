// Package kms implements crypto.Crypto via KMS envelope AES-256-GCM.
//
// Envelope encryption seals data with a short-lived data key (DEK) from KMS
// and stores the KMS-encrypted DEK alongside the ciphertext, so key rotation
// only rotates the KMS key (KeyID/KeyIDs), never re-encrypts stored data.
// Signing and MAC still use local keys (SignKey/Key) when configured.
package kms
