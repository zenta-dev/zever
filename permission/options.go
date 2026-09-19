package permission

import (
	"os"
	"strings"
)

// Options configures checker construction.
type Options struct {
	// Rules lists the authorization rules.
	Rules []Rule
	// ModelPath is the path to the Casbin model file.
	ModelPath string
	// PolicyPath is the path to the Casbin policy file.
	PolicyPath string
	// Roles is a subject-ID to allowed-roles allowlist gating privilege
	// escalation: a subject may only be granted a role (asserted via
	// Subject.Roles at check time) that appears in its list here. It is
	// not a role-hierarchy or inheritance map.
	Roles map[string][]string
}

// Validate checks options for consistency.
func (o Options) Validate() error {
	for _, r := range o.Rules {
		if err := r.Validate(); err != nil {
			return err
		}
	}

	if (o.ModelPath != "") != (o.PolicyPath != "") {
		return &InvalidOptionsError{Reason: "model path and policy path must be set together"}
	}
	if o.ModelPath != "" {
		if strings.Contains(o.ModelPath, "..") {
			return &InvalidOptionsError{Reason: "model path must not contain .."}
		}
		if strings.Contains(o.PolicyPath, "..") {
			return &InvalidOptionsError{Reason: "policy path must not contain .."}
		}
		if _, err := os.Stat(o.ModelPath); err != nil {
			return &InvalidOptionsError{Reason: "model path does not exist"}
		}
		if _, err := os.Stat(o.PolicyPath); err != nil {
			return &InvalidOptionsError{Reason: "policy path does not exist"}
		}
	}

	for k, vs := range o.Roles {
		if k == "" {
			return &InvalidOptionsError{Reason: "role name must be non-empty"}
		}
		if len(k) > 256 {
			return &InvalidOptionsError{Reason: "role name must be at most 256 characters"}
		}
		for _, v := range vs {
			if v == "" {
				return &InvalidOptionsError{Reason: "role value must be non-empty"}
			}
			if len(v) > 256 {
				return &InvalidOptionsError{Reason: "role value must be at most 256 characters"}
			}
		}
	}

	return nil
}
