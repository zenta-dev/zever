package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// pluginValidators maps plugin names to their option validators. It is the
// registry backing per-plugin Validate hooks: file and env layers stay raw
// (unknown option fields pass through), and only Validate interprets the
// options via the registered hook. Access is mutex-guarded so parallel
// tests registering distinct names stay race-free.
var (
	pluginValidatorsMu sync.RWMutex
	pluginValidators   = make(map[string]func(string, json.RawMessage) error)
)

// RegisterPluginValidator registers validate as the Validate hook for the
// named plugin. Validate calls it with the plugin's resolved adapter and
// raw options; a nil error means the plugin's config is valid. Names must
// be unique: re-registering a name fails so two plugins can never silently
// share one validator. A nil validate or empty name also fails.
func RegisterPluginValidator(name string, validate func(adapter string, opts json.RawMessage) error) error {
	if name == "" {
		return errors.New("config: plugin validator name must not be empty")
	}
	if validate == nil {
		return fmt.Errorf("config: plugin validator %q must not be nil", name)
	}
	pluginValidatorsMu.Lock()
	defer pluginValidatorsMu.Unlock()
	if _, dup := pluginValidators[name]; dup {
		return fmt.Errorf("config: duplicate plugin validator %q", name)
	}
	pluginValidators[name] = validate
	return nil
}

// lookupPluginValidator returns the validator registered for name, or false
// when none is registered. Unregistered plugins skip validation: their
// options are opaque to this package.
func lookupPluginValidator(name string) (func(string, json.RawMessage) error, bool) {
	pluginValidatorsMu.RLock()
	defer pluginValidatorsMu.RUnlock()
	validate, ok := pluginValidators[name]
	return validate, ok
}

// sortedPluginNames returns the plugin map's keys in sorted order so
// Validate and RedactedServices iterate deterministically.
func sortedPluginNames(m map[string]Service[json.RawMessage]) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
