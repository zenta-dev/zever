// Package firebase shares Firebase credential validation and app
// construction between the flag and notification adapters.
//
// It validates the service account path and JSON early, never includes
// credential bytes in errors, and performs a single SDK NewApp call.
package firebase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

// Credentials carries the shared Firebase identity. Both adapters convert
// their public options into this struct and delegate validation here,
// preserving their public Options shapes.
type Credentials struct {
	// ProjectID is the Firebase project ID. Required.
	ProjectID string
	// ServiceAccount is the path to the service account JSON key file. Required.
	ServiceAccount string
}

// Validate checks project, path policy, and JSON content early. Missing
// files and invalid JSON fail here so callers fail closed before SDK use.
// Errors carry paths and reasons only, never credential bytes.
func (c Credentials) Validate() error {
	if c.ProjectID == "" {
		return errors.New("firebase: project id is required")
	}
	if c.ServiceAccount == "" {
		return errors.New("firebase: service_account is required")
	}
	if err := ValidateServiceAccountPath(c.ServiceAccount); err != nil {
		return err
	}
	raw, err := os.ReadFile(c.ServiceAccount)
	if err != nil {
		return fmt.Errorf("firebase: service_account path %q cannot be read: %w", c.ServiceAccount, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("firebase: service_account path %q is not valid JSON: %w", c.ServiceAccount, err)
	}
	return nil
}

// ValidateServiceAccountPath enforces a strict key-file path policy:
// non-empty, clean (Clean is a no-op), no ".." element, .json extension,
// and must not resolve to a directory. Missing files pass: readability
// and JSON are checked in Validate or later by the SDK.
func ValidateServiceAccountPath(p string) error {
	if p == "" {
		return errors.New("firebase: service_account is required")
	}
	if filepath.Clean(p) != p {
		return fmt.Errorf("firebase: service_account path %q is not clean", p)
	}
	for _, part := range strings.Split(p, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("firebase: service_account path %q contains traversal", p)
		}
	}
	if !strings.EqualFold(filepath.Ext(p), ".json") {
		return fmt.Errorf("firebase: service_account path %q must have .json extension", p)
	}
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return fmt.Errorf("firebase: service_account path %q is a directory", p)
	}
	return nil
}

// newFirebaseApp is a seam so the SDK error branch is coverable
// hermetically. Production assigns firebase.NewApp, which with a non-nil
// Config never returns an error.
var newFirebaseApp = firebase.NewApp

// NewApp validates credentials then initializes the Firebase app. It makes
// a single SDK call; service follow-ups (RemoteConfig, Messaging) stay in
// the adapters.
func NewApp(ctx context.Context, c Credentials) (*firebase.App, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	app, err := newFirebaseApp(ctx, &firebase.Config{ProjectID: c.ProjectID},
		option.WithAuthCredentialsFile(option.ServiceAccount, c.ServiceAccount))
	if err != nil {
		return nil, fmt.Errorf("firebase: init app: %w", err)
	}
	return app, nil
}
