package flag

import (
	"time"
)

// DefaultTimeout is the default flag operation timeout applied by adapters.
const DefaultTimeout = 30 * time.Second

// StaticOptions configures the static file-backed flag adapter.
type StaticOptions struct {
	// Path is the flag file path. Empty means an empty flag set.
	Path string
	// Reload enables file reloading.
	Reload bool
}

// FirebaseOptions configures the Firebase-backed flag adapter.
type FirebaseOptions struct {
	// ProjectID is the Firebase project ID. Required.
	ProjectID string
	// ServiceAccount is the path to the service account JSON key file. Required.
	ServiceAccount string
	// Timeout is the operation timeout. Zero means the default.
	Timeout time.Duration
}

// Options configures flag client construction.
type Options struct {
	// Static carries the static adapter settings.
	Static StaticOptions
	// Firebase carries the Firebase adapter settings.
	Firebase FirebaseOptions
}

// Validate checks options for consistency.
// Zero Firebase Timeout means "apply default" and is valid; only negative
// values fail. Empty Static Path means an empty flag set and is valid.
// Adapters validate their own required fields.
func (o Options) Validate() error {
	if o.Firebase.Timeout < 0 {
		return &InvalidOptionsError{Reason: "firebase timeout must be >= 0"}
	}
	return nil
}
