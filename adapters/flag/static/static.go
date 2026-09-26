package static

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/log"
)

// DefaultLogInterval caps reload-failure log spam to one line per interval.
const DefaultLogInterval = time.Second

// maxInt and minInt bound float-to-int conversion for the host platform.
const (
	maxInt = int(^uint(0) >> 1)
	minInt = -maxInt - 1
)

// precisionLimit is 2^53: float64 values at or beyond it cannot all
// represent integers exactly, so integer coercion rejects them.
const precisionLimit = float64(1 << 53)

var _ flag.Flag = (*driver)(nil)

// New builds a file-backed flag client from opts.
//
// An empty Static.Path yields an empty flag set without error (local
// development default). Otherwise the path must be clean with no ".."
// element, carry a .json extension, and not be a directory; the file
// must parse as a JSON object. Absolute paths are allowed: the path is
// operator configuration, resolved by the process, not by callers.
func New(opts flag.Options) (flag.Flag, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("static: %w", err)
	}

	path := opts.Static.Path
	if path == "" {
		return &driver{flags: map[string]any{}, logger: opts.Logger}, nil
	}

	if filepath.Clean(path) != path {
		return nil, &flag.InvalidOptionsError{Reason: fmt.Sprintf("static path %q is not clean", path)}
	}
	for _, el := range strings.Split(path, string(filepath.Separator)) {
		if el == ".." {
			return nil, &flag.InvalidOptionsError{Reason: fmt.Sprintf("static path %q contains traversal", path)}
		}
	}
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return nil, &flag.InvalidOptionsError{Reason: fmt.Sprintf("static path %q must have .json extension", path)}
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("static: load %q: %w", path, err)
	}
	if fi.IsDir() {
		return nil, &flag.InvalidOptionsError{Reason: fmt.Sprintf("static path %q is a directory", path)}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("static: load %q: %w", path, err)
	}

	var flags map[string]any
	if err = json.Unmarshal(data, &flags); err != nil {
		return nil, fmt.Errorf("static: load %q: %w", path, err)
	}
	if flags == nil {
		flags = map[string]any{}
	}

	return &driver{
		path:     path,
		reload:   opts.Static.Reload,
		flags:    flags,
		lastMod:  fi.ModTime(),
		lastHash: sha256.Sum256(data),
		logger:   opts.Logger,
	}, nil
}

// driver is a file-backed flag.Flag. Path and reload are immutable
// after New; flags, lastMod, lastHash, and lastLog are guarded by mu.
type driver struct {
	mu       sync.RWMutex
	path     string
	reload   bool
	flags    map[string]any
	lastMod  time.Time
	lastHash [32]byte
	lastLog  time.Time
	logger   log.Logger
}

func (d *driver) log() log.Logger {
	if d != nil && d.logger != nil {
		return d.logger
	}

	return noop.New()
}

// lookup returns the raw value for key. A nil stored value counts as
// missing. Reload failures keep the last-good set and never error.
func (d *driver) lookup(key string) (any, bool) {
	if d.reload && d.path != "" {
		d.reloadIfChanged()
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	v, ok := d.flags[key]
	if !ok || v == nil {
		return nil, false
	}
	return v, true
}

// reloadIfChanged re-reads the file when its modtime advanced. A missing
// file, read error, identical hash, or bad JSON all keep last-good.
func (d *driver) reloadIfChanged() {
	path := d.path

	fi, err := os.Stat(path)
	if err != nil {
		d.rateLimitedLog("flag: static: stat %s failed: %v; keeping last-good flags", path, err)
		return
	}

	d.mu.RLock()
	lastMod := d.lastMod
	lastHash := d.lastHash
	d.mu.RUnlock()

	if !fi.ModTime().After(lastMod) {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		d.rateLimitedLog("flag: static: read %s failed: %v; keeping last-good flags", path, err)
		return
	}

	hash := sha256.Sum256(data)
	if hash == lastHash {
		d.mu.Lock()
		d.lastMod = fi.ModTime()
		d.mu.Unlock()
		return
	}

	var flags map[string]any
	if err := json.Unmarshal(data, &flags); err != nil {
		d.rateLimitedLog("flag: static: reload of %s failed: %v; keeping last-good flags", path, err)
		return
	}
	if flags == nil {
		flags = map[string]any{}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if hash == d.lastHash {
		d.lastMod = fi.ModTime()
		return
	}
	d.flags = flags
	d.lastMod = fi.ModTime()
	d.lastHash = hash
}

// rateLimitedLog emits at most one line per DefaultLogInterval.
func (d *driver) rateLimitedLog(format string, args ...any) {
	d.mu.Lock()
	now := time.Now()
	if now.Sub(d.lastLog) < DefaultLogInterval {
		d.mu.Unlock()
		return
	}
	d.lastLog = now
	d.mu.Unlock()

	d.log().Warn().Msgf(format, args...)
}

// Bool evaluates key as a bool. Strings "true"/"false" (any case)
// coerce; anything else mismatches.
func (d *driver) Bool(ctx context.Context, key string, fallback bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return fallback, err
	}
	if err := flag.ValidateKey(key); err != nil {
		return fallback, fmt.Errorf("static: %w", err)
	}

	v, ok := d.lookup(key)
	if !ok {
		return fallback, nil
	}

	switch b := v.(type) {
	case bool:
		return b, nil
	case string:
		p, perr := parseStrictBool(b)
		if perr != nil {
			return fallback, fmt.Errorf("flag: static: key %q is not a bool", key)
		}
		return p, nil
	default:
		return fallback, fmt.Errorf("flag: static: key %q is not a bool", key)
	}
}

// String evaluates key as a string. JSON numbers format without
// exponents (1000000, not 1e+06); bools render as true/false.
func (d *driver) String(ctx context.Context, key string, fallback string) (string, error) {
	if err := ctx.Err(); err != nil {
		return fallback, err
	}
	if err := flag.ValidateKey(key); err != nil {
		return fallback, fmt.Errorf("static: %w", err)
	}

	v, ok := d.lookup(key)
	if !ok {
		return fallback, nil
	}

	switch s := v.(type) {
	case string:
		return s, nil
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(s), nil
	case bool:
		return strconv.FormatBool(s), nil
	default:
		return fallback, fmt.Errorf("flag: static: key %q is not a string", key)
	}
}

// Int evaluates key as an int. JSON numbers must be exact integers
// within int range and below 2^53; numeric strings parse as float
// first, so "30" works and "3.14" fails.
func (d *driver) Int(ctx context.Context, key string, fallback int) (int, error) {
	if err := ctx.Err(); err != nil {
		return fallback, err
	}
	if err := flag.ValidateKey(key); err != nil {
		return fallback, fmt.Errorf("static: %w", err)
	}

	v, ok := d.lookup(key)
	if !ok {
		return fallback, nil
	}

	switch n := v.(type) {
	case int:
		return n, nil
	case float64:
		return floatToInt(key, fallback, n)
	case string:
		f, ferr := strconv.ParseFloat(n, 64)
		if ferr != nil {
			return fallback, fmt.Errorf("flag: static: key %q is not an int", key)
		}
		return floatToInt(key, fallback, f)
	default:
		return fallback, fmt.Errorf("flag: static: key %q is not an int", key)
	}
}

// JSON decodes key into out. String values decode as raw JSON
// documents; other values round-trip through encoding/json. A missing
// key fills out from fallback when fallback is non-nil.
func (d *driver) JSON(ctx context.Context, key string, out any, fallback any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := flag.ValidateKey(key); err != nil {
		return fmt.Errorf("static: %w", err)
	}
	if out == nil {
		return fmt.Errorf("flag: static: key %q: nil out", key)
	}

	v, ok := d.lookup(key)
	if !ok {
		if fallback == nil {
			return nil
		}
		b, err := json.Marshal(fallback)
		if err != nil {
			return fmt.Errorf("flag: static: key %q: %w", key, err)
		}
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("flag: static: key %q: %w", key, err)
		}
		return nil
	}

	var b []byte
	if s, ok := v.(string); ok {
		b = []byte(s)
	} else {
		var err error
		b, err = json.Marshal(v)
		if err != nil {
			return fmt.Errorf("flag: static: key %q: %w", key, err)
		}
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("flag: static: key %q: %w", key, err)
	}
	return nil
}

// Close releases resources. The static driver holds none, so Close is
// a nil no-op and the client stays usable afterwards.
func (d *driver) Close() error {
	return nil
}

// parseStrictBool accepts only true/false, case-insensitive.
func parseStrictBool(s string) (bool, error) {
	switch {
	case strings.EqualFold(s, "true"):
		return true, nil
	case strings.EqualFold(s, "false"):
		return false, nil
	default:
		return false, errStrictBool
	}
}

// errStrictBool marks strings outside the true/false pair. Callers wrap
// it with the key context.
var errStrictBool = errors.New("flag: static: strict bool parse failed")

// floatToInt converts an exact integer float to int, rejecting
// fractions, imprecise magnitudes (>= 2^53), and out-of-range values.
// The checks are ordered so every branch is reachable: NaN/Inf,
// fraction, host-range overflow, then precision.
func floatToInt(key string, fallback int, n float64) (int, error) {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return fallback, fmt.Errorf("flag: static: key %q is not an int", key)
	}
	if math.Trunc(n) != n {
		return fallback, fmt.Errorf("flag: static: key %q is not an int", key)
	}
	if n < float64(minInt) || n > float64(maxInt) {
		return fallback, fmt.Errorf("flag: static: key %q overflows int", key)
	}
	if math.Abs(n) >= precisionLimit {
		return fallback, fmt.Errorf("flag: static: key %q is not an int", key)
	}
	return int(n), nil
}
