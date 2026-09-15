package firebase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/remoteconfig"
	"google.golang.org/api/option"

	"github.com/zenta-dev/zever/flag"
)

var _ flag.Flag = (*client)(nil)

type client struct {
	tpl     *remoteconfig.ServerTemplate
	eval    func(map[string]any) (*remoteconfig.ServerConfig, error)
	timeout time.Duration
}

// New builds a Firebase Remote Config-backed flag.Flag.
// It validates options, initializes the Firebase app from the service
// account file, and fail-fast loads the server template.
func New(opts flag.Options) (flag.Flag, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("firebase: %w", err)
	}
	fb := opts.Firebase
	if fb.ProjectID == "" {
		return nil, invalidOptions("firebase project id is required")
	}
	if fb.ServiceAccount == "" {
		return nil, invalidOptions("firebase service account is required")
	}
	if err := validateServiceAccountPath(fb.ServiceAccount); err != nil {
		return nil, err
	}

	timeout := fb.Timeout
	if timeout == 0 {
		timeout = flag.DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tpl, err := loadTemplate(ctx, fb.ProjectID, fb.ServiceAccount)
	if err != nil {
		return nil, err
	}
	return &client{tpl: tpl, eval: tpl.Evaluate, timeout: timeout}, nil
}

// loadTemplate initializes the Firebase app from the service account file
// and fail-fast loads the server template. It is a variable so hermetic
// tests can substitute a Set()-initialized template without network or
// credentials.
// loadServerTemplate is a seam so success/failure of tpl.Load can be
// tested hermetically without a live Firebase backend.
var loadServerTemplate = func(ctx context.Context, tpl *remoteconfig.ServerTemplate) error {
	return tpl.Load(ctx)
}

var loadTemplate = func(ctx context.Context, projectID, serviceAccount string) (*remoteconfig.ServerTemplate, error) {
	// NewApp with a non-nil Config never returns an error; all failure
	// paths sit behind config == nil (firebase-admin-go v4.21).
	app, _ := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID},
		option.WithAuthCredentialsFile(option.ServiceAccount, serviceAccount))

	rc, err := app.RemoteConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("firebase: init remote config: %w", err)
	}

	// InitServerTemplate fails solely on unstringifiable default values;
	// an empty map cannot trigger that path (firebase-admin-go v4.21).
	tpl, _ := rc.InitServerTemplate(map[string]any{}, "")
	if err := loadServerTemplate(ctx, tpl); err != nil {
		return nil, fmt.Errorf("firebase: load template: %w", err)
	}
	return tpl, nil
}

func validateServiceAccountPath(p string) error {
	if p == "" {
		return invalidOptions("firebase service account is required")
	}

	cleaned := filepath.Clean(p)
	if cleaned != p {
		return fmt.Errorf("firebase: service_account path %q is not clean", p)
	}

	// Element-wise traversal check: a ".." path element escapes the
	// directory. Substring matching would overmatch names like
	// "foo..bar". Clean paths cannot hide ".." elsewhere: Clean
	// resolves interior dot-dot, so equality above already rejected
	// "a/../b".
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("firebase: service_account path %q contains traversal", p)
		}
	}

	if filepath.Ext(cleaned) != ".json" {
		return fmt.Errorf("firebase: service_account path %q must have .json extension", p)
	}

	if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
		return fmt.Errorf("firebase: service_account path %q is a directory", p)
	}

	return nil
}

// invalidOptions wraps reason as *flag.InvalidOptionsError with firebase
// context. The error-interface conversion keeps vet's %w operand check
// quiet while preserving errors.Is/As through the chain.
func invalidOptions(reason string) error {
	return fmt.Errorf("firebase: %w", error(&flag.InvalidOptionsError{Reason: reason}))
}

func evalContextMap(ctx context.Context) map[string]any {
	m := map[string]any{}
	ec, ok := flag.EvalContextFrom(ctx)
	if !ok {
		return m
	}
	if ec.RandomizationID != "" {
		m["randomizationID"] = ec.RandomizationID
	}
	for k, v := range ec.Signals {
		m[k] = v
	}
	return m
}

func (c *client) evaluate(ctx context.Context, key string) (*remoteconfig.ServerConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := flag.ValidateKey(key); err != nil {
		return nil, err
	}
	cfg, err := c.eval(evalContextMap(ctx))
	if err != nil {
		return nil, fmt.Errorf("firebase: evaluate: %w", err)
	}
	return cfg, nil
}

// Bool evaluates key as a bool. Truthiness follows the Firebase SDK
// (1/true/t/yes/y/on, case-insensitive), a superset of the static adapter.
func (c *client) Bool(ctx context.Context, key string, fallback bool) (bool, error) {
	cfg, err := c.evaluate(ctx, key)
	if err != nil {
		return fallback, err
	}
	if cfg.GetValueSource(key) == remoteconfig.Static {
		return fallback, nil
	}
	return cfg.GetBoolean(key), nil
}

// String evaluates key as a string.
func (c *client) String(ctx context.Context, key string, fallback string) (string, error) {
	cfg, err := c.evaluate(ctx, key)
	if err != nil {
		return fallback, err
	}
	if cfg.GetValueSource(key) == remoteconfig.Static {
		return fallback, nil
	}
	return cfg.GetString(key), nil
}

// Int evaluates key as an int. Float-formatted values truncate toward zero;
// unparseable values return (fallback, error).
func (c *client) Int(ctx context.Context, key string, fallback int) (int, error) {
	cfg, err := c.evaluate(ctx, key)
	if err != nil {
		return fallback, err
	}
	if cfg.GetValueSource(key) == remoteconfig.Static {
		return fallback, nil
	}
	n, err := parseFirebaseInt(cfg.GetString(key))
	if err != nil {
		return fallback, fmt.Errorf("flag: firebase: key %q is not an int", key)
	}
	return n, nil
}

// JSON decodes key into out. A missing key fills out from fallback when
// fallback is non-nil; invalid stored JSON returns an error.
func (c *client) JSON(ctx context.Context, key string, out any, fallback any) error {
	if out == nil {
		return errors.New("flag: firebase: json out is nil")
	}
	cfg, err := c.evaluate(ctx, key)
	if err != nil {
		return err
	}
	if cfg.GetValueSource(key) == remoteconfig.Static {
		if fallback == nil {
			return nil
		}
		raw, err := json.Marshal(fallback)
		if err != nil {
			return fmt.Errorf("flag: firebase: key %q fallback marshal: %w", key, err)
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("flag: firebase: key %q fallback unmarshal: %w", key, err)
		}
		return nil
	}
	if err := json.Unmarshal([]byte(cfg.GetString(key)), out); err != nil {
		return fmt.Errorf("flag: firebase: key %q is not valid JSON: %w", key, err)
	}
	return nil
}

// Close releases associated resources. It is a nil-op: the client holds no
// live connections beyond the load-once template.
func (c *client) Close() error { return nil }

func parseFirebaseInt(raw string) (int, error) {
	if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		return n, nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, errNotAnInt
	}
	t := math.Trunc(f)
	if t > float64(math.MaxInt) || t < float64(math.MinInt) {
		return 0, errNotAnInt
	}
	return int(t), nil
}

// errNotAnInt marks unparseable integer flag values. Callers wrap it with
// the key context.
var errNotAnInt = errors.New("not an int")
