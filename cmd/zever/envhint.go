package main

import (
	"strings"

	"github.com/zenta-dev/zever/config"
)

// serviceEnvFields lists the env field names a battery accepts, without the
// service prefix: "ADAPTER", "DSN", "MAXCONNS", ... The list comes from the
// same Options struct the .env.example generator renders, so a hint always
// names keys the project file already lists.
func serviceEnvFields(battery string) []string {
	opts := config.DefaultOptions(battery, "")
	if opts == nil {
		return nil
	}

	var b strings.Builder

	appendEnvKeys(&b, "X", "_", opts)

	fields := []string{"ADAPTER"}

	for line := range strings.Lines(b.String()) {
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}

		fields = append(fields, strings.TrimPrefix(line[:i], "X_"))
	}

	return fields
}

// serviceEnvKeys lists the canonical env keys a battery accepts, under the
// given prefix: "DB" for the default instance, "DB__REPORTING" for a named
// one.
func serviceEnvKeys(prefix, battery string) []string {
	fields := serviceEnvFields(battery)
	if len(fields) == 0 {
		return nil
	}

	prefix = strings.ToUpper(prefix)

	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, prefix+"_"+f)
	}

	return out
}

// foldEnvField folds an option name to its comparison form: uppercase with
// every "_" and " " dropped, mirroring config's own case-insensitive,
// separator-insensitive field matching.
func foldEnvField(s string) string {
	var b strings.Builder

	for _, r := range strings.ToUpper(s) {
		if r == '_' || r == ' ' {
			continue
		}

		b.WriteRune(r)
	}

	return b.String()
}
