package config

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func mustRegisterPlugin(t *testing.T, name string, validate func(string, json.RawMessage) error) {
	t.Helper()
	if err := RegisterPluginValidator(name, validate); err != nil {
		t.Fatalf("RegisterPluginValidator(%q): %v", name, err)
	}
}

func mustMergePlugins(t *testing.T, cfg *Config, raw map[string]ServiceConfig) {
	t.Helper()
	if err := merge(cfg, raw); err != nil {
		t.Fatalf("merge plugins: %v", err)
	}
}

func pluginRaw(t *testing.T, m map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal plugin options: %v", err)
	}
	return json.RawMessage(b)
}

func TestPluginMergeRawPassthrough(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	cfg := Default()
	mustMergePlugins(t, cfg, map[string]ServiceConfig{
		"plugins": {Options: map[string]any{
			"plugintest_merge": map[string]any{
				"adapter": "custom",
				"options": map[string]any{
					"anything_goes": 1,
					"nested":        map[string]any{"a": "b"},
				},
			},
		}},
	})
	svc, ok := cfg.Plugins["plugintest_merge"]
	if !ok {
		t.Fatal("missing plugintest_merge")
	}
	if svc.Adapter != "custom" {
		t.Fatalf("adapter = %q, want custom", svc.Adapter)
	}
	var opts map[string]any
	if err := json.Unmarshal(svc.Options, &opts); err != nil {
		t.Fatalf("unmarshal plugin options: %v", err)
	}
	if opts["anything_goes"] != float64(1) {
		t.Fatalf("options lost unknown fields: %v", opts)
	}
}

func TestPluginMergeEmptyKeeps(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	cfg := Default()
	mustMergePlugins(t, cfg, map[string]ServiceConfig{
		"plugins": {Options: map[string]any{
			"plugintest_keep": map[string]any{
				"adapter": "custom",
				"options": map[string]any{"k": "v"},
			},
		}},
	})
	before := string(cfg.Plugins["plugintest_keep"].Options)
	mustMergePlugins(t, cfg, map[string]ServiceConfig{
		"plugins": {Options: map[string]any{
			"plugintest_keep": map[string]any{},
		}},
	})
	svc := cfg.Plugins["plugintest_keep"]
	if svc.Adapter != "custom" {
		t.Fatalf("empty merge wiped adapter: %q", svc.Adapter)
	}
	if string(svc.Options) != before {
		t.Fatalf("empty merge wiped options: %s", svc.Options)
	}
}

func TestPluginMergeStrictEnvelope(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	err := merge(Default(), map[string]ServiceConfig{
		"plugins": {Options: map[string]any{
			"plugintest_badenvelope": map[string]any{"adaptre": "x"},
		}},
	})
	if err == nil {
		t.Fatal("want error for unknown envelope key")
	}
	var derr *DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("want *DecodeError, got %T: %v", err, err)
	}
}

func TestPluginMergeUnknownServiceStillStrict(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	cfg := Default()
	mustMergePlugins(t, cfg, map[string]ServiceConfig{
		"plugins": {Options: map[string]any{
			"plugintest_other": map[string]any{"adapter": "x"},
		}},
	})
	err := merge(cfg, map[string]ServiceConfig{"nope": {Adapter: "x"}})
	var unknown *UnknownServiceError
	if !errors.As(err, &unknown) {
		t.Fatalf("want UnknownServiceError, got %v", err)
	}
}

func TestPluginMergeEmptyBlockNoop(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	cfg := Default()
	mustMergePlugins(t, cfg, map[string]ServiceConfig{"plugins": {}})
	if cfg.Plugins != nil {
		t.Fatalf("empty plugins block initialized map: %v", cfg.Plugins)
	}
}

func TestPluginEnvAdapterOverride(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	cfg := Default()
	mustMergePlugins(t, cfg, map[string]ServiceConfig{
		"plugins": {Options: map[string]any{
			"plugintest_env": map[string]any{"adapter": "file-adapter"},
		}},
	})
	t.Setenv("PLUGINTEST_ENV_ADAPTER", "env-adapter")
	if err := applyEnv(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Plugins["plugintest_env"].Adapter != "env-adapter" {
		t.Fatalf("adapter = %q, want env-adapter", cfg.Plugins["plugintest_env"].Adapter)
	}
}

func TestPluginEnvNoPluginsNoop(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	t.Setenv("PLUGINTEST_MISSING_ADAPTER", "x")
	cfg := Default()
	if err := applyEnv(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Plugins != nil {
		t.Fatalf("env created plugins: %v", cfg.Plugins)
	}
}

func TestPluginValidateSuccess(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	mustRegisterPlugin(t, "plugintest_valid_ok", func(adapter string, opts json.RawMessage) error {
		if adapter != "custom" {
			return errors.New("want adapter custom")
		}
		var m map[string]any
		if err := json.Unmarshal(opts, &m); err != nil {
			return err
		}
		if m["k"] != "v" {
			return errors.New("want k=v")
		}
		return nil
	})
	cfg := Default()
	cfg.Plugins = map[string]Service[json.RawMessage]{
		"plugintest_valid_ok": {Adapter: "custom", Options: pluginRaw(t, map[string]any{"k": "v"})},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestPluginValidateFailure(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	mustRegisterPlugin(t, "plugintest_valid_fail", func(string, json.RawMessage) error {
		return errors.New("boom")
	})
	cfg := Default()
	cfg.Plugins = map[string]Service[json.RawMessage]{
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
	cfg.Plugins = map[string]Service[json.RawMessage]{
		"plugintest_unregistered_xyz": {Adapter: "whatever", Options: pluginRaw(t, map[string]any{"any": 1})},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unregistered plugin must skip validation: %v", err)
	}
}

func TestPluginDuplicateValidator(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	mustRegisterPlugin(t, "plugintest_dup", func(string, json.RawMessage) error { return nil })
	err := RegisterPluginValidator("plugintest_dup", func(string, json.RawMessage) error { return nil })
	if err == nil {
		t.Fatal("want duplicate registration error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %v, want duplicate mention", err)
	}
	if err := RegisterPluginValidator("", func(string, json.RawMessage) error { return nil }); err == nil {
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
	cfg.Plugins = map[string]Service[json.RawMessage]{
		"plugintest_redact": {
			Adapter: "custom",
			Options: pluginRaw(t, map[string]any{"api_key": "s3cret", "endpoint": "https://example.com"}),
		},
	}
	got := cfg.RedactedServices()
	if len(got) != 35 {
		t.Fatalf("got %d services, want 35", len(got))
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

func TestPluginLoadEndToEnd(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	mustRegisterPlugin(t, "plugintest_load", func(adapter string, _ json.RawMessage) error {
		if adapter != "custom" {
			return errors.New("want adapter custom")
		}
		return nil
	})
	dir := t.TempDir()
	p := writeFile(t, dir, "zever.yaml",
		"plugins:\n"+
			"  plugintest_load:\n"+
			"    adapter: custom\n"+
			"    options:\n"+
			"      api_key: s3cret\n")
	t.Setenv("PLUGINTEST_LOAD_ADAPTER", "custom")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	svc, ok := cfg.Plugins["plugintest_load"]
	if !ok {
		t.Fatal("missing plugintest_load after Load")
	}
	if svc.Adapter != "custom" {
		t.Fatalf("adapter = %q", svc.Adapter)
	}
	got := cfg.RedactedServices()["plugintest_load"]
	if got.Options["api_key"] != RedactedValue {
		t.Fatalf("api_key not redacted: %v", got.Options["api_key"])
	}
}
