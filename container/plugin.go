package container

import (
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/shared/registry"
)

// PluginAPIVersion is the only plugin build-function ABI the container
// accepts. RegisterPlugin rejects any other version so a plugin compiled
// against a different container contract fails fast at registration
// instead of misbehaving at resolve time.
const PluginAPIVersion = 1

// ErrPluginNilBuild reports a nil build function passed to RegisterPlugin.
var ErrPluginNilBuild = errors.New("container: plugin build func is nil")

// pluginEntry holds the per-Container lazy slot for one plugin name.
// Failed builds are not cached: lazy.get retries on the next Resolve.
type pluginEntry struct {
	slot lazy[any]
}

var pluginRegistry = registry.New[string, func(*config.Config) (any, error)](
	ErrPluginNilBuild,
	func(name string) error { return fmt.Errorf("container: plugin %q already registered", name) },
	func(name string) error { return fmt.Errorf("container: plugin %q not registered", name) },
)

// RegisterPlugin registers a named plugin factory for later Resolve.
// The factory runs lazily on first Resolve per Container and receives that
// Container's *config.Config. Registration is process-wide and
// goroutine-safe via the shared registry.
func RegisterPlugin[T any](name string, version int, build func(*config.Config) (T, error)) error {
	if name == "" {
		return errors.New("container: plugin name must not be empty")
	}

	if version != PluginAPIVersion {
		return fmt.Errorf("container: plugin %q version mismatch: got %d, want %d", name, version, PluginAPIVersion)
	}

	if build == nil {
		return ErrPluginNilBuild
	}

	return pluginRegistry.Register(name, func(cfg *config.Config) (any, error) {
		v, err := build(cfg)
		if err != nil {
			return nil, err
		}

		return any(v), nil
	})
}

// Resolve looks up name and lazily builds its instance for c on first call.
// Successful builds are cached per Container; failed builds retry on the
// next call. A stored value that does not implement T fails with a
// type-mismatch error. An unregistered name fails with a not-registered error.
func Resolve[T any](c *Container, name string) (T, error) {
	var zero T

	if c == nil {
		return zero, errors.New("container: resolve on nil container")
	}

	factory, err := pluginRegistry.Lookup(name)
	if err != nil {
		return zero, err
	}

	c.pluginsMu.Lock()
	if c.plugins == nil {
		c.plugins = make(map[string]*pluginEntry)
	}
	entry, ok := c.plugins[name]
	if !ok {
		entry = &pluginEntry{}
		c.plugins[name] = entry
	}
	c.pluginsMu.Unlock()

	v, err := entry.slot.get(func() (any, error) {
		return factory(c.cfg)
	})
	if err != nil {
		return zero, fmt.Errorf("container: plugin %q: %w", name, err)
	}

	typed, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("container: plugin %q type mismatch: stored %T, want %T", name, v, zero)
	}

	return typed, nil
}

// pluginCloseSnapshot names one resolved plugin instance for Close.
type pluginCloseSnapshot struct {
	name string
	v    any
	ok   bool
}

// pluginSnapshots returns resolved plugin instances without opening new ones.
func (c *Container) pluginSnapshots() []pluginCloseSnapshot {
	c.pluginsMu.Lock()
	defer c.pluginsMu.Unlock()

	snaps := make([]pluginCloseSnapshot, 0, len(c.plugins))
	for name, entry := range c.plugins {
		v, ok := entry.slot.getIfResolved()
		if !ok {
			continue
		}
		snaps = append(snaps, pluginCloseSnapshot{name: name, v: v, ok: true})
	}

	return snaps
}
