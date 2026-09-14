package s3core

import (
	"fmt"

	"github.com/zenta-dev/zever/storage"
)

// ValidatePolicyCoherence rejects policies where public write and update differ. A static unsigned PUT URL allows both create and overwrite, so write and update public access must match.
func ValidatePolicyCoherence(prefix string, policies map[storage.BucketName]storage.Policy, def storage.Policy) error {
	errFor := func(scope string, p storage.Policy) error {
		if p.Public(storage.PermWrite) && !p.Public(storage.PermUpdate) {
			return fmt.Errorf(
				"[%s] policy %s has public write but restricted update; "+
					"a static unsigned PUT URL also allows anonymous overwrites of existing keys, "+
					"so update must be public too",
				prefix, scope,
			)
		}

		if p.Public(storage.PermUpdate) && !p.Public(storage.PermWrite) {
			return fmt.Errorf(
				"[%s] policy %s has public update but restricted write; "+
					"a static unsigned PUT URL also allows anonymous creation of new keys, "+
					"so write must be public too",
				prefix, scope,
			)
		}

		return nil
	}
	for bucket, p := range policies {
		if err := errFor(fmt.Sprintf("bucket %q", bucket), p); err != nil {
			return err
		}
	}

	return errFor("default policy", def)
}
