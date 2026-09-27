package s3

import (
	"github.com/zenta-dev/zever/core/storage"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = storage.Register(storage.AdapterS3, New)
}
