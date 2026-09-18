// Package media stores media assets with swappable adapters.
//
// It handles upload, download, ranged download, stat, probe, transform, and
// delete with ID validation and size caps. It does not serve HTTP or run
// transcoders itself beyond invoking configured ffmpeg tooling.
//
// Type safety: Media interface with typed Options plus Adapter enum plus
// Factory. Asset, Info, Probe, TransformOps, UploadOptions, and MediaKind plus
// ValidID, GenerateID, and extension helpers. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with
// zero-infra defaults for tests, with the local adapter using the filesystem
// only. Config file plus env MEDIA_ADAPTER (no prefix, e.g. MEDIA_ADAPTER).
// See config/README.md.
//
// Container: container.New(cfg) then c.Media(). Lazy per-service singleton,
// retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Upload, Download,
// DownloadRange, Delete, Stat, Probe, and Transform take ctx first. Close
// releases backend resources and takes no ctx. Only resolved services close.
//
// Errors: sentinel errors, errors.Is compatible, prefixed media:. Name service
// and field only in errors, with DuplicateAdapterError, UnknownAdapterError,
// InvalidAdapterError, InvalidOptionsError, NotFoundError, InvalidIDError,
// SizeLimitError, UnsupportedFormatError, InvalidTransformError, and
// InvalidRangeError carrying IDs, sizes, formats, or reasons.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// AccessKeyID and SecretAccessKey are never logged. ValidID rejects traversal
// such as empty, dot, and dot-dot forms. Endpoints must include scheme and
// host. Presigned TTL is capped at 7 days.
//
// Performance: single downloads capped at 64MB by default. Decoded images
// capped at 50M pixels. Derived TTL defaults to 10m and presign TTL to 1h.
// Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init
// wiring.
//
// Example: see ExampleOpen in example_test.go.
package media
