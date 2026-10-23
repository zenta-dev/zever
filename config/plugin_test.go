package config

import (
	"encoding/json/jsontext"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/shared/codec"
)

func mustRegisterPlugin(t *testing.T, name string, validate func(string, jsontext.Value) error) {
	t.Helper()
	if err := RegisterPluginValidator(name, validate); err != nil {
		t.Fatalf("RegisterPluginValidator(%q): %v", name, err)
	}
	// The validator registry is process-global; drop the entry when the
	// test ends so the next -count iteration can register the same name.
	t.Cleanup(func() {
		pluginValidatorsMu.Lock()
		defer pluginValidatorsMu.Unlock()
		delete(pluginValidators, name)
	})
}

func pluginRaw(t *testing.T, m map[string]any) jsontext.Value {
	t.Helper()
	b, err := codec.Marshal(m)
	if err != nil {
		t.Fatalf("marshal plugin options: %v", err)
	}
	return jsontext.Value(b)
}

func TestPluginOptionsStayRaw(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Plugins = map[string]Service[jsontext.Value]{
		"plugintest_merge": {
			Adapter: "custom",
			Options: pluginRaw(t, map[string]any{
				"anything_goes": 1,
				"nested":        map[string]any{"a": "b"},
			}),
		},
	}
	svc, ok := cfg.Plugins["plugintest_merge"]
	if !ok {
		t.Fatal("missing plugintest_merge")
	}
	if svc.Adapter != "custom" {
		t.Fatalf("adapter = %q, want custom", svc.Adapter)
	}
	var opts map[string]any
	if err := codec.Unmarshal(svc.Options, &opts); err != nil {
		t.Fatalf("unmarshal plugin options: %v", err)
	}
	if opts["anything_goes"] != float64(1) {
		t.Fatalf("options lost unknown fields: %v", opts)
	}
}

func TestPluginEnvAdapterOverride(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Plugins = map[string]Service[jsontext.Value]{
		"plugintest_env": {Adapter: "default-adapter", Options: pluginRaw(t, map[string]any{"k": "v"})},
	}
	applyPluginEnv(Env{"PLUGINTEST_ENV_ADAPTER": "env-adapter"}, cfg)
	if cfg.Plugins["plugintest_env"].Adapter != "env-adapter" {
		t.Fatalf("adapter = %q, want env-adapter", cfg.Plugins["plugintest_env"].Adapter)
	}
	if string(cfg.Plugins["plugintest_env"].Options) != `{"k":"v"}` {
		t.Fatalf("env wiped options: %s", cfg.Plugins["plugintest_env"].Options)
	}
}

func TestPluginEnvNoPluginsNoop(t *testing.T) {
	t.Parallel()
	cfg := Default()
	applyPluginEnv(Env{"PLUGINTEST_MISSING_ADAPTER": "x"}, cfg)
	if cfg.Plugins != nil {
		t.Fatalf("env created plugins: %v", cfg.Plugins)
	}
}

func TestPluginValidateSuccess(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	mustRegisterPlugin(t, "plugintest_valid_ok", func(adapter string, opts jsontext.Value) error {
		if adapter != "custom" {
			return errors.New("want adapter custom")
		}
		var m map[string]any
		if err := codec.Unmarshal(opts, &m); err != nil {
			return err
		}
		if m["k"] != "v" {
			return errors.New("want k=v")
		}
		return nil
	})
	cfg := Default()
	cfg.Plugins = map[string]Service[jsontext.Value]{
		"plugintest_valid_ok": {Adapter: "custom", Options: pluginRaw(t, map[string]any{"k": "v"})},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestPluginValidateFailure(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	mustRegisterPlugin(t, "plugintest_valid_fail", func(string, jsontext.Value) error {
		return errors.New("boom")
	})
	cfg := Default()
	cfg.Plugins = map[string]Service[jsontext.Value]{
		"plugintest_valid_fail": {Adapter: "custom"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "config: invalid plugintest_valid_fail options: boom") {
		t.Fatalf("error = %v, want wrapped plugin failure", err)
	}
}

func TestPluginValidateUnregisteredSkips(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	cfg := Default()
	cfg.Plugins = map[string]Service[jsontext.Value]{
		"plugintest_unregistered_xyz": {Adapter: "whatever", Options: pluginRaw(t, map[string]any{"any": 1})},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unregistered plugin must skip validation: %v", err)
	}
}

func TestPluginDuplicateValidator(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	mustRegisterPlugin(t, "plugintest_dup", func(string, jsontext.Value) error { return nil })
	err := RegisterPluginValidator("plugintest_dup", func(string, jsontext.Value) error { return nil })
	if err == nil {
		t.Fatal("want duplicate registration error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %v, want duplicate mention", err)
	}
	if err := RegisterPluginValidator("", func(string, jsontext.Value) error { return nil }); err == nil {
		t.Fatal("want empty-name error")
	}
	if err := RegisterPluginValidator("plugintest_nil", nil); err == nil {
		t.Fatal("want nil-validator error")
	}
}

func TestPluginRedactedServices(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	cfg := Default()
	cfg.Plugins = map[string]Service[jsontext.Value]{
		"plugintest_redact": {
			Adapter: "custom",
			Options: pluginRaw(t, map[string]any{"api_key": "s3cret", "endpoint": "https://example.com"}),
		},
	}
	got := cfg.RedactedServices()
	if len(got) != 38 {
		t.Fatalf("got %d services, want 38", len(got))
	}
	sc, ok := got["plugintest_redact"]
	if !ok {
		t.Fatal("missing plugintest_redact")
	}
	if sc.Adapter != "custom" {
		t.Fatalf("adapter = %q", sc.Adapter)
	}
	if sc.Options["api_key"] != RedactedValue {
		t.Fatalf("api_key not redacted: %v", sc.Options["api_key"])
	}
	if sc.Options["endpoint"] != "https://example.com" {
		t.Fatalf("endpoint mutated: %v", sc.Options["endpoint"])
	}
}

func TestPluginEnvEndToEnd(t *testing.T) {
	t.Parallel()
	mustRegisterPlugin(t, "plugintest_load", func(adapter string, _ jsontext.Value) error {
		if adapter != "custom" {
			return errors.New("want adapter custom")
		}
		return nil
	})
	cfg := Default()
	cfg.Plugins = map[string]Service[jsontext.Value]{
		"plugintest_load": {Adapter: "default", Options: pluginRaw(t, map[string]any{"api_key": "s3cret"})},
	}
	applyPluginEnv(Env{"PLUGINTEST_LOAD_ADAPTER": "custom"}, cfg)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	svc, ok := cfg.Plugins["plugintest_load"]
	if !ok {
		t.Fatal("missing plugintest_load")
	}
	if svc.Adapter != "custom" {
		t.Fatalf("adapter = %q", svc.Adapter)
	}
	got := cfg.RedactedServices()["plugintest_load"]
	if got.Options["api_key"] != RedactedValue {
		t.Fatalf("api_key not redacted: %v", got.Options["api_key"])
	}
}
