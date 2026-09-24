package config

import (
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// durationType identifies time.Duration fields, which stay strings on the
// wire ("5s" style) and parse via time.ParseDuration instead of integer
// coercion.
var durationType = reflect.TypeOf(time.Duration(0))

// applyEnv overlays environment variables onto cfg with no prefix:
// DB_ADAPTER selects the db adapter, DB_MAXCONNS sets a db field.
//
// Matching is best-effort by design: a variable is ignored unless its head
// (up to the first "_") names a known service and the remainder is non-empty
// and resolves to a settable field. Unknown variables never error, so
// unrelated process environment (PATH, HOME, AUTH_TOKEN, DB, USER, ...) can
// never break startup. Strict mode would fail here: bare names like PATH
// and HOME have no "_" at all, and plausible heads like AUTH in AUTH_TOKEN
// collide with real service names.
//
// Field lookup is case-insensitive with "_" ignored, so MAX_RETRIES matches
// MaxRetries. One nesting level is supported (AUTH_JWT_SECRET sets
// Options.JWT.Secret). Durations stay strings and parse via
// time.ParseDuration ("5s" style). Adapter assignment is unchecked here;
// Validate rejects bad names fail-closed later. Reflection runs only for
// env-provided fields; the file path stays marshal-based (decodeOptions).
//
// Reflection errors name the FIELD only — values are never echoed, so env
// file secrets cannot leak into error text.
func applyEnv(cfg *Config) error {
	for _, kv := range os.Environ() {
		key, value, _ := strings.Cut(kv, "=")
		head, rest, ok := strings.Cut(key, "_")
		if !ok {
			continue
		}
		svc := strings.ToLower(head)
		field := strings.ToLower(rest)
		if field == "" {
			continue
		}
		adapter, opts, ok := serviceRefs(cfg, svc)
		if !ok {
			continue
		}
		if field == "adapter" {
			*adapter = value
			continue
		}
		if err := setOptionField(svc, opts, []string{field}, value); err != nil {
			var unknown *UnknownFieldError
			if !errors.As(err, &unknown) {
				return &InvalidOptionsError{Service: svc, Reason: err.Error()}
			}
			if !strings.Contains(field, "_") {
				continue
			}
			outer, inner, _ := strings.Cut(field, "_")
			if err := setOptionField(svc, opts, []string{outer, inner}, value); err != nil {
				var nested *UnknownFieldError
				if errors.As(err, &nested) {
					continue
				}
				return &InvalidOptionsError{Service: svc, Reason: err.Error()}
			}
		}
	}
	return nil
}

// serviceRefs returns pointers to the named service's adapter and options.
// ok is false for unknown names.
func serviceRefs(cfg *Config, svc string) (adapter *string, opts any, ok bool) {
	switch svc {
	case "ai":
		return &cfg.AI.Adapter, &cfg.AI.Options, true
	case "analytics":
		return &cfg.Analytics.Adapter, &cfg.Analytics.Options, true
	case "auth":
		return &cfg.Auth.Adapter, &cfg.Auth.Options, true
	case "billing":
		return &cfg.Billing.Adapter, &cfg.Billing.Options, true
	case "cache":
		return &cfg.Cache.Adapter, &cfg.Cache.Options, true
	case "crypto":
		return &cfg.Crypto.Adapter, &cfg.Crypto.Options, true
	case "db":
		return &cfg.DB.Adapter, &cfg.DB.Options, true
	case "document":
		return &cfg.Document.Adapter, &cfg.Document.Options, true
	case "eventbus":
		return &cfg.EventBus.Adapter, &cfg.EventBus.Options, true
	case "flag":
		return &cfg.Flag.Adapter, &cfg.Flag.Options, true
	case "geo":
		return &cfg.Geo.Adapter, &cfg.Geo.Options, true
	case "i18n":
		return &cfg.I18n.Adapter, &cfg.I18n.Options, true
	case "idempotency":
		return &cfg.Idempotency.Adapter, &cfg.Idempotency.Options, true
	case "lock":
		return &cfg.Lock.Adapter, &cfg.Lock.Options, true
	case "log":
		return &cfg.Log.Adapter, &cfg.Log.Options, true
	case "mailer":
		return &cfg.Mailer.Adapter, &cfg.Mailer.Options, true
	case "media":
		return &cfg.Media.Adapter, &cfg.Media.Options, true
	case "notification":
		return &cfg.Notification.Adapter, &cfg.Notification.Options, true
	case "observability":
		return &cfg.Observability.Adapter, &cfg.Observability.Options, true
	case "password":
		return &cfg.Password.Adapter, &cfg.Password.Options, true
	case "payment":
		return &cfg.Payment.Adapter, &cfg.Payment.Options, true
	case "permission":
		return &cfg.Permission.Adapter, &cfg.Permission.Options, true
	case "queue":
		return &cfg.Queue.Adapter, &cfg.Queue.Options, true
	case "ratelimit":
		return &cfg.RateLimit.Adapter, &cfg.RateLimit.Options, true
	case "router":
		return &cfg.Router.Adapter, &cfg.Router.Options, true
	case "scheduler":
		return &cfg.Scheduler.Adapter, &cfg.Scheduler.Options, true
	case "search":
		return &cfg.Search.Adapter, &cfg.Search.Options, true
	case "secrets":
		return &cfg.Secrets.Adapter, &cfg.Secrets.Options, true
	case "session":
		return &cfg.Session.Adapter, &cfg.Session.Options, true
	case "storage":
		return &cfg.Storage.Adapter, &cfg.Storage.Options, true
	case "tenant":
		return &cfg.Tenant.Adapter, &cfg.Tenant.Options, true
	case "vectorstore":
		return &cfg.VectorStore.Adapter, &cfg.VectorStore.Options, true
	case "webhook":
		return &cfg.Webhook.Adapter, &cfg.Webhook.Options, true
	case "workflow":
		return &cfg.Workflow.Adapter, &cfg.Workflow.Options, true
	default:
		return nil, nil, false
	}
}

// normName lowercases and strips "_" so SNAKE and CamelCase spellings meet:
// "max_retries" and "MaxRetries" both normalize to "maxretries".
func normName(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "_", "")
}

// jsonFieldName reports the encoding/json key for a struct field: the tag
// name when present, otherwise the Go field name.
func jsonFieldName(sf reflect.StructField) string {
	if tag, ok := sf.Tag.Lookup("json"); ok {
		if name, _, _ := strings.Cut(tag, ","); name != "" {
			return name
		}
	}
	return sf.Name
}

// fieldCandidates lists v's exported field names (JSON key, embedded
// structs' fields promoted), for use as closest's candidate list when
// reporting an UnknownFieldError with a "did you mean %q?" suggestion.
func fieldCandidates(v reflect.Value) []string {
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()

	var names []string

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}

		if sf.Anonymous && v.Field(i).Kind() == reflect.Struct {
			names = append(names, fieldCandidates(v.Field(i))...)

			continue
		}

		names = append(names, jsonFieldName(sf))
	}

	return names
}

// findField locates an exported settable field of struct v by normalized
// JSON name, descending into embedded structs (encoding/json promotion).
func findField(v reflect.Value, name string) (reflect.Value, bool) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		f := v.Field(i)
		if sf.Anonymous && f.Kind() == reflect.Struct {
			if nested, ok := findField(f, name); ok {
				return nested, true
			}
			continue
		}
		if normName(jsonFieldName(sf)) == normName(name) {
			return f, true
		}
	}
	return reflect.Value{}, false
}

// setOptionField sets one env-provided value on the options struct dst.
// path holds one segment (flat or promoted field) or two (outer struct,
// inner field). Lookup misses fail with UnknownFieldError (caller ignores
// them); only value TYPE failures return plain errors (caller wraps them
// in InvalidOptionsError). Slices, maps, funcs, interfaces, and other
// complex kinds are rejected as unknown — they have no string form.
func setOptionField(service string, dst any, path []string, value string) error {
	field := strings.Join(path, ".")
	// Indirect unwraps one pointer level (dst is *Options; nested fields
	// may be *struct). It returns an invalid Value for nil pointers and
	// returns non-pointers unchanged, so validity + struct-kind + settability
	// together accept exactly *struct without naming pointer kinds.
	root := reflect.Indirect(reflect.ValueOf(dst))
	if !root.IsValid() || root.Kind() != reflect.Struct || !root.CanSet() {
		return &UnknownFieldError{Service: service, Field: field}
	}
	cur := root
	for _, name := range path[:len(path)-1] {
		f, ok := findField(cur, name)
		if !ok {
			return &UnknownFieldError{Service: service, Field: field, Suggestion: closest(name, fieldCandidates(cur))}
		}
		f = reflect.Indirect(f)
		if !f.IsValid() || f.Kind() != reflect.Struct {
			return &UnknownFieldError{Service: service, Field: field}
		}
		cur = f
	}
	leaf, ok := findField(cur, path[len(path)-1])
	if !ok {
		return &UnknownFieldError{Service: service, Field: field, Suggestion: closest(path[len(path)-1], fieldCandidates(cur))}
	}
	return setScalar(service, field, leaf, value)
}

// setScalar coerces the env string into f by kind. Durations parse via
// time.ParseDuration ("5s" style). Failures name the field only via
// scrubbedValue — the raw value is never echoed, so secrets in env/file
// values cannot leak into error text (decodeOptions already scrubs the
// file path the same way).
func setScalar(service, field string, f reflect.Value, value string) error {
	miss := func() error { return &UnknownFieldError{Service: service, Field: field} }
	if f.Type() == durationType {
		d, err := time.ParseDuration(value)
		if err != nil {
			return scrubbedValue(field)
		}
		f.SetInt(int64(d))
		return nil
	}
	switch f.Kind() { //nolint:exhaustive // default rejects every kind this env parser doesn't support
	case reflect.String:
		f.SetString(value)
		return nil
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return scrubbedValue(field)
		}
		f.SetBool(b)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, f.Type().Bits())
		if err != nil {
			return scrubbedValue(field)
		}
		f.SetInt(n)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, f.Type().Bits())
		if err != nil {
			return scrubbedValue(field)
		}
		f.SetUint(n)
		return nil
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(value, f.Type().Bits())
		if err != nil {
			return scrubbedValue(field)
		}
		f.SetFloat(n)
		return nil
	default:
		return miss()
	}
}

// scrubbedValue reports an unparseable env value by field name only.
// strconv and time parse errors quote the offending input, which may be a
// secret, so only the field name survives.
func scrubbedValue(field string) error {
	return errors.New("config: invalid value for field " + strconv.Quote(field))
}
