package vault

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zenta-dev/zever/core/secrets"
)

// DefaultHTTPTimeout bounds Vault HTTP round-trips.
const DefaultHTTPTimeout = 5 * time.Second

// driver implements secrets.Secrets against Vault KVv2 over HTTP.
type driver struct {
	baseURL   string
	token     string
	mount     string
	namespace string
	client    *http.Client
}

// New creates a Vault KVv2 secrets adapter from validated Options.
func New(opts Options) (secrets.Secrets, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	token, err := opts.resolveToken()
	if err != nil {
		return nil, err
	}
	return &driver{
		baseURL:   strings.TrimRight(strings.TrimSpace(opts.Addr), "/"),
		token:     token,
		mount:     strings.Trim(strings.TrimSpace(opts.Mount), "/"),
		namespace: strings.TrimSpace(opts.Namespace),
		client:    &http.Client{Timeout: DefaultHTTPTimeout},
	}, nil
}

func (d *driver) request(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var buf *bytes.Buffer
	if body == nil {
		buf = bytes.NewBuffer(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("vault: encode request: %w", err)
		}
		buf = bytes.NewBuffer(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.baseURL+path, buf)
	if err != nil {
		return nil, fmt.Errorf("vault: build request: %w", err)
	}
	req.Header.Set("X-Vault-Token", d.token)
	if d.namespace != "" {
		req.Header.Set("X-Vault-Namespace", d.namespace)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (d *driver) dataPath(name string) string {
	return "/v1/" + d.mount + "/data/" + url.PathEscape(name)
}

func (d *driver) metadataPath(name string) string {
	return "/v1/" + d.mount + "/metadata/" + url.PathEscape(name)
}

// Get returns the secret value for name.
func (d *driver) Get(ctx context.Context, name string) ([]byte, error) {
	if err := secrets.ValidateName(name); err != nil {
		return nil, err
	}
	req, err := d.request(ctx, http.MethodGet, d.dataPath(name), nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault: get %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("vault: get %s: %w", name, secrets.ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault: get %s: unexpected status %d", name, resp.StatusCode)
	}
	var out struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("vault: get %s: decode: %w", name, err)
	}
	raw, ok := out.Data.Data["value"]
	if !ok {
		return nil, fmt.Errorf("vault: get %s: %w", name, secrets.ErrNotFound)
	}
	// Set always base64-encodes, so decode strictly: a value that does not
	// decode was written by a foreign or corrupted client and must never be
	// returned raw.
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("vault: get %s: %w: %w", name, ErrInvalidBase64, err)
	}
	return decoded, nil
}

// Set stores value under name.
func (d *driver) Set(ctx context.Context, name string, value []byte) error {
	if err := secrets.ValidateName(name); err != nil {
		return err
	}
	body := map[string]any{
		"data": map[string]string{"value": base64.StdEncoding.EncodeToString(value)},
	}
	req, err := d.request(ctx, http.MethodPut, d.dataPath(name), body)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault: set %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("vault: set %s: unexpected status %d", name, resp.StatusCode)
	}
	return nil
}

// Delete removes the secret stored under name.
func (d *driver) Delete(ctx context.Context, name string) error {
	if err := secrets.ValidateName(name); err != nil {
		return err
	}
	req, err := d.request(ctx, http.MethodDelete, d.metadataPath(name), nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault: delete %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("vault: delete %s: %w", name, secrets.ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("vault: delete %s: unexpected status %d", name, resp.StatusCode)
	}
	return nil
}

// List returns the names of all stored secrets.
func (d *driver) List(ctx context.Context) ([]string, error) {
	req, err := d.request(ctx, http.MethodGet, "/v1/"+d.mount+"/metadata/?list=true", nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault: list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault: list: unexpected status %d", resp.StatusCode)
	}
	var out struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("vault: list: decode: %w", err)
	}
	return out.Data.Keys, nil
}

// Close releases no resources and always succeeds.
func (d *driver) Close(_ context.Context) error {
	return nil
}
