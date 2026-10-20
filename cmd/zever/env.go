package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/zenta-dev/zever/config"
)

// resolveServiceEnv builds the service Env for CLI commands: .env/.env.local
// discovery plus the process environment (see config.FromEnv), with an
// optional explicit dotenv file layered between them: the explicit file wins
// over discovered dotfiles, the process wins over everything. A missing
// explicit file is an error; missing discovered files are skipped silently.
//
// The explicit file's ${VAR} references expand against that file only;
// process-referencing expansion belongs in the discovered .env files.
func resolveServiceEnv(configPath string) (config.Env, error) {
	env, err := config.FromEnv()
	if err != nil {
		return nil, err
	}
	if configPath == "" {
		return env, nil
	}
	extra, err := config.LoadEnv(configPath)
	if err != nil {
		return nil, fmt.Errorf("load dotenv file %q: %w", configPath, err)
	}
	proc := processKeySet()
	for k, v := range extra {
		if !proc[strings.ToUpper(k)] {
			env[k] = v
		}
	}
	return env, nil
}

// resolveServiceConfig resolves the validated service Config for CLI
// commands: the Env from resolveServiceEnv applied over service defaults.
func resolveServiceConfig(configPath string) (*config.Config, error) {
	env, err := resolveServiceEnv(configPath)
	if err != nil {
		return nil, err
	}
	return config.Resolve(env)
}

// wrapEnvConfigError wraps a config resolution failure with the exact-key
// hint when the failure names an unknown env field, so every command that
// resolves service config from the environment points at the offending key
// the same way (see envTypoHint).
func wrapEnvConfigError(tag string, env config.Env, err error) error {
	if hint := envTypoHint(env, err); hint != "" {
		return fmt.Errorf("%s: %w\n%s", tag, err, hint)
	}

	return fmt.Errorf("%s: %w", tag, err)
}

// processKeySet snapshots the process environment's uppercased keys so
// explicit dotenv files never override a real process variable.
func processKeySet() map[string]bool {
	out := make(map[string]bool)
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		out[strings.ToUpper(key)] = true
	}
	return out
}
