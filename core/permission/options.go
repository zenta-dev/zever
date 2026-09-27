package permission

import (
	"os"
	"strings"
)

// Options configures checker construction.
type Options struct {
	// Rules lists the authorization rules.
	Rules []Rule `json:"rules" toml:"rules" yaml:"rules"`
	// ModelPath is the path to the Casbin model file.
	ModelPath string `json:"model_path" toml:"model_path" yaml:"model_path"`
	// PolicyPath is the path to the Casbin policy file.
	PolicyPath string `json:"policy_path" toml:"policy_path" yaml:"policy_path"`
	// Roles is a subject-ID to allowed-roles allowlist gating privilege
	// escalation: a subject may only be granted a role (asserted via
	// Subject.Roles at check time) that appears in its list here. It is
	// not a role-hierarchy or inheritance map.
	Roles map[string][]string `json:"roles" toml:"roles" yaml:"roles"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	for _, r := range o.Rules {
		if err := r.Validate(); err != nil {
			return err
		}
	}

	if (o.ModelPath != "") != (o.PolicyPath != "") {
		return &InvalidOptionsError{Reason: "model_path and policy_path must be set together"}
	}
	if o.ModelPath != "" {
		if strings.Contains(o.ModelPath, "..") {
			return &InvalidOptionsError{Reason: "model_path must not contain .."}
		}
		if strings.Contains(o.PolicyPath, "..") {
			return &InvalidOptionsError{Reason: "policy_path must not contain .."}
		}
		if _, err := os.Stat(o.ModelPath); err != nil {
			return &InvalidOptionsError{Reason: "model_path does not exist"}
		}
		if _, err := os.Stat(o.PolicyPath); err != nil {
			return &InvalidOptionsError{Reason: "policy_path does not exist"}
		}
	}

	for k, vs := range o.Roles {
		if k == "" {
			return &InvalidOptionsError{Reason: "role_name must be non-empty"}
		}
		if len(k) > 256 {
			return &InvalidOptionsError{Reason: "role_name must be at most 256 characters"}
		}
		for _, v := range vs {
			if v == "" {
				return &InvalidOptionsError{Reason: "role_value must be non-empty"}
			}
			if len(v) > 256 {
				return &InvalidOptionsError{Reason: "role_value must be at most 256 characters"}
			}
		}
	}

	return nil
}
