// Package auth defines a generic authentication facade with swappable backends.
//
// Standalone companions: auth never wires the password or crypto packages.
// They are opt-in batteries for services that need them alongside auth:
//
//   - password.Hasher checks stored credentials on the login path. Reach
//     for it when a backend must verify a password before issuing a token.
//     Constructor: argon2.New, opened via password.Register and
//     password.Open with password.AdapterArgon2ID.
//   - crypto.Crypto encrypts tokens or other secrets at rest. Load the key
//     from secrets (for example secrets.Open with secrets.Env), then open
//     the backend via crypto.Register and crypto.Open with
//     crypto.AdapterLocal (constructor local.New).
//
// Example:
//
//	password.Register(password.AdapterArgon2ID, argon2.New)
//	hasher, err := password.Open(password.AdapterArgon2ID, password.Options{})
//	if err != nil {
//		return err
//	}
//	ok, err := hasher.Verify(ctx, storedHash, suppliedPassword)
//	if err != nil || !ok {
//		return ErrInvalidCredentials
//	}
//
//	crypto.Register(crypto.AdapterLocal, local.New)
//	enc, err := crypto.Open(crypto.AdapterLocal, crypto.Options{Key: keyFromSecrets})
//	if err != nil {
//		return err
//	}
//	sealed, err := enc.Encrypt(ctx, []byte(token.Value))
package auth
