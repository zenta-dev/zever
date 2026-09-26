package adapters

import (
	"github.com/zenta-dev/zever/flag"
	flagfirebase "github.com/zenta-dev/zever/flag/firebase"
	"github.com/zenta-dev/zever/media"
	medias3 "github.com/zenta-dev/zever/media/s3"
	"github.com/zenta-dev/zever/storage"
	storager2 "github.com/zenta-dev/zever/storage/r2"
	storages3 "github.com/zenta-dev/zever/storage/s3"
)

// RegisterCloud registers the cloud-backed adapters the core container no
// longer imports: storage/s3 and storage/r2 (both AWS-SDK based, sharing
// internal/s3opts), media/s3, and flag/firebase (Firebase Remote Config).
// The local storage adapter stays wired by the container. Registration only
// fills factory maps; it performs no I/O. Duplicate registrations are
// ignored, so calling RegisterCloud more than once (or alongside
// RegisterAll) is safe.
func RegisterCloud() {
	_ = flag.Register(flag.Firebase, flagfirebase.New)

	_ = media.Register(media.S3, medias3.New)

	_ = storage.Register(storage.AdapterS3, storages3.New)
	_ = storage.Register(storage.AdapterR2, storager2.New)
}
