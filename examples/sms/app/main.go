// Command app shows the SMS plugin dogfood example: config plus container
// plugin registration plus Resolve plus Send.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/examples/sms"
	"github.com/zenta-dev/zever/examples/sms/stub"
)

// pluginName is the cfg.Plugins key and container plugin name.
const pluginName = "sms"

func run(ctx context.Context) error {
	cfg := config.Default()
	cfg.Plugins = map[string]config.Service[json.RawMessage]{
		pluginName: {
			Adapter: "stub",
			Options: json.RawMessage(`{"from":"+15550000"}`),
		},
	}

	if err := stub.Register(); err != nil {
		return fmt.Errorf("register sms stub: %w", err)
	}

	if err := container.RegisterPlugin[sms.SMS](pluginName, container.PluginAPIVersion, func(cfg *config.Config) (sms.SMS, error) {
		entry, ok := cfg.Plugins[pluginName]
		if !ok {
			return nil, fmt.Errorf("sms plugin: missing plugins[%q] entry", pluginName)
		}

		var opts sms.Options

		if len(entry.Options) > 0 {
			if err := json.Unmarshal(entry.Options, &opts); err != nil {
				return nil, fmt.Errorf("sms plugin: decode options: %w", err)
			}
		}

		adapter, err := sms.ParseAdapter(entry.Adapter)
		if err != nil {
			return nil, err
		}

		return sms.Open(adapter, opts)
	}); err != nil {
		return fmt.Errorf("register sms plugin: %w", err)
	}

	c := container.New(cfg)

	svc, err := container.Resolve[sms.SMS](c, pluginName)
	if err != nil {
		return fmt.Errorf("resolve sms plugin: %w", err)
	}

	if err := svc.Send(ctx, "+15550001", "hello from sms plugin"); err != nil {
		return fmt.Errorf("sms send: %w", err)
	}

	if err := svc.Close(ctx); err != nil {
		return fmt.Errorf("sms close: %w", err)
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
