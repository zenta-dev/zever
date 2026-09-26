package flag

import (
	"time"

	"github.com/zenta-dev/zever/core/log"
)

// DefaultTimeout is the default flag operation timeout applied by adapters.
const DefaultTimeout = 30 * time.Second

// StaticOptions configures the static file-backed flag adapter.
type StaticOptions struct {
	// Path is the flag file path. Empty means an empty flag set.
	Path string `json:"path" toml:"path" yaml:"path"`
	// Reload enables file reloading.
	Reload bool `json:"reload" toml:"reload" yaml:"reload"`
}

// FirebaseOptions configures the Firebase-backed flag adapter.
type FirebaseOptions struct {
	// ProjectID is the Firebase project ID. Required.
	ProjectID string `json:"project_id" toml:"project_id" yaml:"project_id"`
	// ServiceAccount is the path to the service account JSON key file. Required.
	ServiceAccount string `json:"service_account" toml:"service_account" yaml:"service_account"`
	// Timeout is the operation timeout. Zero means the default.
	Timeout time.Duration `json:"timeout" toml:"timeout" yaml:"timeout"`
}

// Options configures flag client construction.
type Options struct {
	// Static carries the static adapter settings.
	Static StaticOptions `json:"static" toml:"static" yaml:"static"`
	// Firebase carries the Firebase adapter settings.
	Firebase FirebaseOptions `json:"firebase" toml:"firebase" yaml:"firebase"`
	// Logger emits reload warnings. Defaults to a no-op logger when nil.
	Logger log.Logger `json:"-" toml:"-" yaml:"-"`
}

// Validate checks options for consistency, joining all violations.
//
// Validate checks options for consistency.
// Zero Firebase Timeout means "apply default" and is valid; only negative
// values fail. Empty Static Path means an empty flag set and is valid.
// Adapters validate their own required fields.
func (o Options) Validate() error {
	if o.Firebase.Timeout < 0 {
		return &InvalidOptionsError{Reason: "firebase_timeout must be >= 0"}
	}
	return nil
}
