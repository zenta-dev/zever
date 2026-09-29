package examples_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/shared/registry"
)

// Minimal third-party battery mirroring the external-sms plugin pattern:
// small interface, string adapter, validating options, Register/Open over
// shared/registry, deterministic stub driver, then container plugin wiring.

// greeter is the battery interface hosts resolve from the container.
type greeter interface {
	Hello(ctx context.Context, name string) string
}

// greetAdapter identifies the greeter backend implementation.
type greetAdapter string

// stubGreet selects the deterministic in-memory greeter backend.
const stubGreet greetAdapter = "stub"

// greetPluginName is the container plugin name.
const greetPluginName = "greet-docs-example"

// greetOptions configures greeter construction.
type greetOptions struct {
	Prefix string
}

// validate checks options for consistency.
func (o greetOptions) validate() error {
	if o.Prefix == "" {
		return errors.New("greet: prefix must be non-empty")
	}
	return nil
}

// greetFactory creates a greeter from options.
type greetFactory func(opts greetOptions) (greeter, error)

var greetFactories = registry.New[greetAdapter, greetFactory](
	errors.New("greet: nil factory"),
	func(adapter greetAdapter) error { return fmt.Errorf("greet: duplicate registration: %s", adapter) },
	func(adapter greetAdapter) error { return fmt.Errorf("greet: unknown adapter: %s", adapter) },
)

// registerGreet associates an adapter with a factory for later use by openGreet.
func registerGreet(adapter greetAdapter, factory greetFactory) error {
	if factory == nil {
		return fmt.Errorf("greet: nil factory for adapter %s", adapter)
	}
	return greetFactories.Register(adapter, factory)
}

// openGreet creates a greeter for adapter using the registered factory and opts.
func openGreet(adapter greetAdapter, opts greetOptions) (greeter, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	factory, err := greetFactories.Lookup(adapter)
	if err != nil {
		return nil, err
	}
	g, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("greet: open %s: %w", adapter, err)
	}
	return g, nil
}

var _ greeter = (*greetDriver)(nil)

// greetDriver is the deterministic in-memory greeter backend.
type greetDriver struct {
	prefix string
}

// newGreetStub validates opts and returns a stub greeter backend.
func newGreetStub(opts greetOptions) (greeter, error) {
	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("greet stub: %w", err)
	}
	return &greetDriver{prefix: opts.Prefix}, nil
}

// Hello renders the greeting for name.
func (d *greetDriver) Hello(_ context.Context, name string) string {
	return d.prefix + " " + name
}

// registerGreetPlugin wires the full path in one call: adapter registration
// plus the versioned container plugin build func. Call once at startup
// before container.Resolve.
func registerGreetPlugin() error {
	if err := registerGreet(stubGreet, newGreetStub); err != nil {
		return fmt.Errorf("greet: register stub: %w", err)
	}
	if err := container.RegisterPlugin[greeter](greetPluginName, container.PluginAPIVersion, func(_ *config.Config) (greeter, error) {
		return openGreet(stubGreet, greetOptions{Prefix: "hi"})
	}); err != nil {
		return fmt.Errorf("greet: register container plugin: %w", err)
	}
	return nil
}

// ExamplePlugin_registerResolve mirrors the external-sms plugin pattern at
// minimal size: register the stub battery as a container plugin, resolve it,
// and call it.
func Example_pluginRegisterResolve() {
	if err := registerGreetPlugin(); err != nil {
		fmt.Println("register error")
		return
	}
	c := container.New(nil)
	svc, err := container.Resolve[greeter](c, greetPluginName)
	if err != nil {
		fmt.Println("resolve error")
		return
	}
	fmt.Println(svc.Hello(context.Background(), "ada"))

	again, err := container.Resolve[greeter](c, greetPluginName)
	if err != nil {
		fmt.Println("second resolve error")
		return
	}
	fmt.Println(again.Hello(context.Background(), "grace"))
	// Output:
	// hi ada
	// hi grace
}
