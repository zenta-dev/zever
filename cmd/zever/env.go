package main

import (
	"fmt"

	"github.com/zenta-dev/zever/config"
)

// resolveServiceEnv builds the service Env for CLI commands: .env/.env.local
// discovery plus the process environment (see config.FromEnv).
//
// There is no file-path input: service config is env-only, so the CLI reads
// the dotenv files it discovers in the working directory and lets the
// process environment win. A config file named on the command line is
// flag misuse, not a supported input.
func resolveServiceEnv() (config.Env, error) {
	return config.FromEnv()
}

// resolveServiceConfig resolves the validated service Config for CLI
// commands: the Env from resolveServiceEnv applied over service defaults.
func resolveServiceConfig() (*config.Config, error) {
	env, err := resolveServiceEnv()
	if err != nil {
		return nil, err
	}
	return config.Resolve(env)
}

// wrapEnvConfigError wraps a config resolution failure with the exact-key
// hint when the failure names an unknown env field, so every command that
// resolves service config from the environment points at the offending key
// the same way (see envTypoHint). The hint is a hint like every other: it
// stays out of the way under ZEVER_NO_HINT or --quiet.
func wrapEnvConfigError(tag string, env config.Env, err error) error {
	if shouldShowHint() {
		if hint := envTypoHint(env, err); hint != "" {
			return fmt.Errorf("%s: %w\n%s", tag, err, hint)
		}
	}

	return fmt.Errorf("%s: %w", tag, err)
}
