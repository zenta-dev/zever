package vault

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

// Options holds typed configuration for the vault secrets adapter.
type Options struct {
	// Addr is the Vault server address. Required.
	Addr string `json:"addr" toml:"addr" yaml:"addr"`
	// Token is the Vault authentication token.
	Token string `json:"token" toml:"token" yaml:"token"`
	// TokenFile reads the Vault token from a file when Token is empty.
	TokenFile string `json:"token_file" toml:"token_file" yaml:"token_file"`
	// Mount is the Vault KV mount path. Required.
	Mount string `json:"mount" toml:"mount" yaml:"mount"`
	// Namespace is the Vault namespace.
	Namespace string `json:"namespace" toml:"namespace" yaml:"namespace"`
	// AllowInsecure permits a plain-http Addr for test setups (httptest
	// servers speak http only). Default false: Addr must use https, since
	// the token travels in the X-Vault-Token header and plain http sends
	// it in cleartext. Enabling this logs a warning at open.
	AllowInsecure bool `json:"allow_insecure" toml:"allow_insecure" yaml:"allow_insecure"`
}

// Validate checks Options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if strings.TrimSpace(o.Addr) == "" {
		errs = append(errs, errors.New("vault: addr is required"))
	} else {
		u, err := url.Parse(o.Addr)
		switch {
		case err != nil || u.Scheme == "":
			errs = append(errs, errors.New("vault: addr must include scheme"))
		case u.Scheme != "https" && u.Scheme != "http":
			errs = append(errs, errors.New("vault: addr scheme must be https or http"))
		case u.Scheme == "http" && !o.AllowInsecure:
			errs = append(errs, errors.New("vault: addr must use https (set allow_insecure for plain http test setups)"))
		}
		if err == nil && u.Host == "" {
			errs = append(errs, errors.New("vault: addr must include host"))
		}
	}

	if strings.TrimSpace(o.Mount) == "" {
		errs = append(errs, errors.New("vault: mount is required"))
	}
	if strings.Contains(o.Mount, " ") || strings.Contains(o.Mount, "..") {
		errs = append(errs, errors.New("vault: mount must not contain spaces or dot-dot"))
	}

	token := strings.TrimSpace(o.Token)
	if token == "" && strings.TrimSpace(o.TokenFile) != "" {
		b, err := os.ReadFile(o.TokenFile)
		if err != nil {
			errs = append(errs, errors.New("vault: token_file unreadable"))
		} else if strings.TrimSpace(string(b)) == "" {
			errs = append(errs, errors.New("vault: token_file must not be empty"))
		}
	}
	if token == "" && strings.TrimSpace(o.TokenFile) == "" {
		errs = append(errs, errors.New("vault: token or token_file is required"))
	}

	return errors.Join(errs...)
}

// resolveToken returns Token, or the trimmed contents of TokenFile when Token is empty.
func (o Options) resolveToken() (string, error) {
	if strings.TrimSpace(o.Token) != "" {
		return strings.TrimSpace(o.Token), nil
	}
	b, err := os.ReadFile(o.TokenFile)
	if err != nil {
		return "", errors.New("vault: token_file unreadable")
	}
	tok := strings.TrimSpace(string(b))
	if tok == "" {
		return "", errors.New("vault: token_file must not be empty")
	}
	return tok, nil
}
