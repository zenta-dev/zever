// Command app shows the SMS plugin dogfood example: config plus container
// Provide plus Get plus Send.
package main

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"os"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/examples/sms"
	"github.com/zenta-dev/zever/examples/sms/stub"
	"github.com/zenta-dev/zever/shared/codec"
	"github.com/zenta-dev/zever/shared/ref"
)

// ErrPluginConfig indicates a missing or malformed plugin configuration entry.
var ErrPluginConfig = errors.New("sms: plugin config")

// pluginName is the cfg.Plugins key and container service name.
const pluginName = "sms"

// SMSRef names the wired SMS instance. The app chooses the backend by which
// constructor the Provide entry calls, then rebuilds to change it.
var SMSRef = ref.Default[sms.SMS, sms.Options](pluginName)

func run(ctx context.Context) error {
	cfg := config.Default()
	cfg.Plugins = map[string]config.Service[jsontext.Value]{
		pluginName: {
			Options: jsontext.Value(`{"from":"+15550000"}`),
		},
	}

	entry, ok := cfg.Plugins[pluginName]
	if !ok {
		return fmt.Errorf("sms: %w: missing plugins[%q] entry", ErrPluginConfig, pluginName)
	}

	var opts sms.Options

	if len(entry.Options) > 0 {
		if err := codec.Unmarshal(entry.Options, &opts); err != nil {
			return fmt.Errorf("sms: decode options: %w", err)
		}
	}

	// The backend is selected in code: this app wires the stub backend
	// directly, then rebuilds to change it.
	c := container.New(cfg, container.Provide(SMSRef, stub.Open, container.WithOptions(func(o *sms.Options) {
		*o = opts
	})))

	svc, err := container.Get(c, SMSRef)
	if err != nil {
		return fmt.Errorf("sms: resolve sms: %w", err)
	}

	if err := svc.Send(ctx, "+15550001", "hello from sms plugin"); err != nil {
		return fmt.Errorf("sms: send: %w", err)
	}

	if err := svc.Close(ctx); err != nil {
		return fmt.Errorf("sms: close: %w", err)
	}

	fmt.Println("sms sent via stub plugin")

	return nil
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "sms app:", err)
		os.Exit(1)
	}
}
