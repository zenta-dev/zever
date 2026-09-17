// Package password defines a generic password-hashing facade with swappable adapters.
//
// Standalone battery: no other package (including auth) depends on password.
// It is opt-in. Reach for password.Hasher on the verification path, when a
// login flow must check a supplied password against a stored hash before a
// token is issued.
//
// Example:
//
//	password.Register(password.AdapterArgon2ID, argon2.New)
//	hasher, err := password.Open(password.AdapterArgon2ID, password.Options{})
//	if err != nil {
//		return err
//	}
//	ok, err := hasher.Verify(ctx, storedHash, suppliedPassword)
package password
