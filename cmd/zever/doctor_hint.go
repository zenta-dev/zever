package main

import (
	"strings"
)

// doctorFixHint returns the one-line next action for a failing battery
// row: the exact env key to set when the error names a missing option,
// otherwise the command that shows the battery's resolved values.
//
// The hint never invents a key: a key is only named when the error names a
// field that the battery's own Options struct actually declares, so the
// hint always points at a key the project's .env.example already lists.
func doctorFixHint(battery, errText string) string {
	switch battery {
	case "config":
		// The config row is the whole config's validation, not one
		// battery: the error names the service whose options are wrong.
		return "fix the service env the error names, then re-run 'zever doctor'"
	}

	if key := doctorEnvKey(battery, doctorMissingField(errText)); key != "" {
		return "set " + key + " in .env, then re-run 'zever doctor'"
	}

	return "run 'zever config show' to see the resolved " + battery + " values"
}

// doctorMissingField extracts the option field a battery error reports as
// missing: "api_key is required" yields "api_key", `option "dsn" is
// required` yields "dsn", "jwt secret is required" yields "jwt secret".
// It returns "" when the error names no missing field.
func doctorMissingField(errText string) string {
	const requiredTail = " is required"

	i := strings.LastIndex(errText, requiredTail)
	if i < 0 {
		return ""
	}

	field := strings.TrimSpace(errText[:i])

	// Adapters prefix the field with their own name ("postgres: option
	// \"dsn\"") and the container prefixes that with the service
	// ("container: db: ..."), so keep only the last clause.
	if j := strings.LastIndex(field, ": "); j >= 0 {
		field = strings.TrimSpace(field[j+2:])
	}

	field = strings.TrimPrefix(field, "option ")
	field = strings.Trim(field, `"`)

	if field == "" {
		return ""
	}

	return field
}

// doctorEnvKey returns the env key that carries the named option field for
// the battery, or "" when the battery declares no such field. The key comes
// from the same Options struct the .env.example generator renders, so the
// hint names the exact key the project's .env file already lists.
func doctorEnvKey(battery, field string) string {
	want := foldEnvField(field)
	if want == "" {
		return ""
	}

	for _, f := range serviceEnvFields(battery) {
		if foldEnvField(f) == want {
			return strings.ToUpper(battery) + "_" + f
		}
	}

	return ""
}
