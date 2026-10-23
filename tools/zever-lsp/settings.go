package main

import (
	"encoding/json"
	"math"
	"time"

	"go.lsp.dev/protocol"
)

// Settings is the server's resolved editing configuration, parsed from the
// `workspace/didChangeConfiguration` payload the client sends (nested JSON,
// values the server reads). Every field defaults to today's behavior, so an
// empty payload is identical to no configuration at all.
type Settings struct {
	// ImplicitModuleHints gates the "(module: public)" inlay hint family.
	ImplicitModuleHints bool
	// ErrorCaseHints gates the errors-case status inlay hint family.
	ErrorCaseHints bool
	// DebounceDelay is how long an edit burst must settle before recompiling.
	DebounceDelay time.Duration
}

// defaultSettings returns the configuration that preserves the server's
// pre-settings behavior exactly.
func defaultSettings() Settings {
	return Settings{
		ImplicitModuleHints: true,
		ErrorCaseHints:      true,
		DebounceDelay:       debounceDelay,
	}
}

// Settings JSON keys, as clients nest them under the sole `zever` section:
//
//	{
//	  "zever": {
//	    "inlayHints": { "implicitModule": true, "errorCases": true },
//	    "diagnostics": { "debounceMs": 250 }
//	  }
//	}
const (
	settingsSection       = "zever"
	settingsInlayHintsKey = "inlayHints"
	settingsImplicitKey   = "implicitModule"
	settingsErrorCasesKey = "errorCases"
	settingsDiagKey       = "diagnostics"
	settingsDebounceKey   = "debounceMs"
)

// parseSettings resolves a didChangeConfiguration settings payload against
// the defaults. Replacement semantics: didChangeConfiguration carriers in
// every mainstream client send the whole settings object, not a delta, so a
// family absent from a well-formed payload falls back to its default. Types
// the payload gets wrong (a string where a bool belongs, a debounceMs that
// is not a positive finite number) are ignored per-key, not rejected.
//
// ok=false means the payload is empty or not a JSON object and the caller
// should leave the current settings untouched; ok=true with an absent
// `zever` section yields plain defaults (the client has none configured).
func parseSettings(raw protocol.LSPAny) (parsed Settings, ok bool) {
	parsed = defaultSettings()

	var obj map[string]any

	if len(raw) == 0 {
		return Settings{}, false
	}

	if err := json.Unmarshal(raw, &obj); err != nil {
		return Settings{}, false
	}

	section, ok := jsonObj(obj[settingsSection])
	if !ok {
		return parsed, true
	}

	if hints, hintsOK := jsonObj(section[settingsInlayHintsKey]); hintsOK {
		if b, bOK := jsonBool(hints[settingsImplicitKey]); bOK {
			parsed.ImplicitModuleHints = b
		}

		if b, bOK := jsonBool(hints[settingsErrorCasesKey]); bOK {
			parsed.ErrorCaseHints = b
		}
	}

	if diags, diagsOK := jsonObj(section[settingsDiagKey]); diagsOK {
		if ms, msOK := jsonFloat(diags[settingsDebounceKey]); msOK && ms > 0 {
			parsed.DebounceDelay = time.Duration(ms * float64(time.Millisecond))
		}
	}

	return parsed, true
}

// jsonBool narrows a decoded JSON value to a bool, reporting whether it was one.
func jsonBool(v any) (bool, bool) {
	b, ok := v.(bool)

	return b, ok
}

// jsonFloat narrows a decoded JSON value (numbers decode as float64) to a
// finite one, reporting whether it was; NaN/Inf cannot be produced by a
// conforming JSON document but are rejected anyway for defense in depth.
func jsonFloat(v any) (float64, bool) {
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}

	return f, true
}

// jsonObj narrows a decoded JSON value to an object, reporting whether it was one.
func jsonObj(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)

	return m, ok
}
