// Package crypto defines a generic cryptography facade with swappable adapters.
//
// Standalone battery: no other package (including auth) depends on crypto.
// It is opt-in. Reach for it when tokens or other secrets must be encrypted
// at rest; load the key from secrets (for example secrets.Open with
// secrets.Env) rather than hard-coding it.
//
// Example:
//
//	crypto.Register(crypto.AdapterLocal, local.New)
//	enc, err := crypto.Open(crypto.AdapterLocal, crypto.Options{Key: keyFromSecrets})
//	if err != nil {
//		return err
//	}
//	sealed, err := enc.Encrypt(ctx, plaintext)
package crypto
