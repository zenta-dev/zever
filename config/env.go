package config

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Env is an injectable process environment: variable name to value. Library
// code takes Env as an argument and never touches the real process
// environment itself, so tests pass literal maps; only FromEnv reads
// os.Environ and .env files.
type Env map[string]string

// ErrInvalidValue is returned when an env-provided value cannot be coerced
// into its field. Only the field name is reported: values may be secrets.
var ErrInvalidValue = errors.New("config: invalid value")

// StrictEnvKey is the environment variable that turns strict unknown-key
// rejection back on. With it set to a true value ("1", "true"), every
// unknown key under a wired service prefix fails resolution again, the
// v0.8 behavior. Without it, only near-miss typos fail: an unknown key that
// is close to no known field is an unrelated ambient variable and is
// ignored.
//
// The switch is read from the same env map ApplyEnv is given, so enabling
// it needs no CLI flag and no call-site change.
const StrictEnvKey = "ZEVER_CONFIG_STRICT"

// strictEnv reports whether env opts into strict unknown-key rejection.
// Any case spelling of StrictEnvKey with a true value enables it; a false
// or unparsable value leaves strict mode off, like every other env read.
func strictEnv(env Env) bool {
	for key, value := range env {
		if !strings.EqualFold(key, StrictEnvKey) {
			continue
		}
		on, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil && on {
			return true
		}
	}
	return false
}

// durationType identifies time.Duration fields, which stay strings on the
// wire ("5s" style) and parse via time.ParseDuration instead of integer
// coercion.
var durationType = reflect.TypeOf(time.Duration(0))

// Resolve builds a Config from env: service defaults, overlaid by the
// default instance of every service (`SERVICE_FIELD`, `SERVICE_ADAPTER`),
// then validated fail-closed. Unknown service heads and bare names are
// ignored so ambient process environment never breaks startup. Under a
// wired service prefix an unknown key fails only when it is close to a
// known field (a typo, reported with a did-you-mean hint); a key that is
// close to no known field is an unrelated ambient variable and is ignored.
// Set StrictEnvKey in env to reject every unknown key instead. Named
// instances (`SERVICE__NAME__*`) are left for the caller: enumerate them
// with InstanceNames and apply each with ApplyEnv.
//
// On ANY failure Resolve returns nil plus the error.
func Resolve(env Env) (*Config, error) {
	cfg := Default()
	overlay := stripAdapterKeys(env)
	for _, svc := range knownServiceNames() {
		adapter, opts, ok := serviceRefs(cfg, svc)
		if !ok {
			continue
		}
		if v, ok := lookupEnvKey(env, strings.ToUpper(svc)+"_ADAPTER"); ok {
			*adapter = v
		}
		if err := ApplyEnv(overlay, svc, "", opts); err != nil {
			return nil, err
		}
	}
	applyPluginEnv(env, cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// stripAdapterKeys copies env minus every `SERVICE_ADAPTER` key: adapter
// selection is applied directly by Resolve, so bare-Options ApplyEnv never
// sees it (an Options struct with its own adapter-shaped field keeps its
// default, matching the old file+env layering).
func stripAdapterKeys(env Env) Env {
	out := make(Env, len(env))
	adapters := adapterEnvKeys()

outer:
	for key, value := range env {
		upper := strings.ToUpper(key)
		for _, adapter := range adapters {
			if upper == adapter {
				continue outer
			}
		}
		out[key] = value
	}
	return out
}

// adapterEnvKeys returns the uppercased SERVICE_ADAPTER key of every known
// service. The set is built once per call: the ambient environment holds
// hundreds of unrelated keys, and none of them needs the service list
// rebuilt for every key.
func adapterEnvKeys() []string {
	names := knownServiceNames()
	out := make([]string, 0, len(names))

	for _, svc := range names {
		out = append(out, strings.ToUpper(svc)+"_ADAPTER")
	}

	return out
}

// lookupEnvKey finds key (already uppercased) in env case-insensitively,
// reporting the value. When several cases of the same key exist, the last
// in sorted order wins, keeping multi-case environments deterministic.
func lookupEnvKey(env Env, key string) (string, bool) {
	var (
		found bool
		value string
		best  string
	)

	for k, v := range env {
		if strings.ToUpper(k) != key {
			continue
		}

		// Sorted order wins: the greatest matching key is the last one
		// in sorted order.
		if !found || k > best {
			best, value, found = k, v, true
		}
	}

	return value, found
}

// sortedEnvKeys returns env's keys in sorted order so multi-key application
// and error reporting stay deterministic across map iteration order.
func sortedEnvKeys(env Env) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// prefixedEnvKeys returns the env keys that belong to the wired prefix, in
// sorted order. Filtering before sorting keeps the walk proportional to the
// service's own keys instead of the whole environment: a service owns a
// handful of keys, while the process environment holds hundreds of
// unrelated ones. The order matches sortedEnvKeys over the whole
// environment, so the diagnostics stay deterministic.
func prefixedEnvKeys(env Env, prefix string, named bool) []string {
	var keys []string

	for key := range env {
		if _, ok := applyPrefixKey(key, prefix, named); !ok {
			continue
		}

		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// ApplyEnv overlays env onto dst, the SINGLE reflection point for
// environment configuration. service names the battery (matched
// case-insensitively, so "DB" and "db" agree); name selects the instance,
// "" for the default. The wired prefix is SERVICE for the default instance
// and SERVICE__NAME for a named one, uppercased; keys outside the prefix
// are ignored, as are bare names and other services' keys, so ambient
// process environment never breaks startup.
//
// dst must be a non-nil pointer to a struct, in one of two shapes: a
// Service envelope (an Adapter string field plus an Options struct field,
// as in Config) or bare Options. In envelope form a trailing ADAPTER key
// selects the adapter and every other key descends into Options; in bare
// form ADAPTER keys resolve like any other field and unresolvable ones are
// ignored, since adapter selection is the caller's job there.
//
// Field matching reuses the zero-alloc ASCII-fold: case-insensitive with
// "_" ignored, so MAX_RETRIES matches MaxRetries. One nesting level is
// supported (AUTH_JWT_SECRET sets JWT.Secret). A key matching both a flat
// field and a nested path is a collision and fails. An unknown key under the
// wired prefix fails with an UnknownFieldError carrying a did-you-mean
// suggestion when it is close to a known field (a typo); a key close to no
// known field is an unrelated ambient variable and is ignored, unless
// StrictEnvKey is set in env, which rejects every unknown key. Durations
// parse via time.ParseDuration ("5s" style). Errors name fields only —
// values are never echoed.
func ApplyEnv(env Env, service, name string, dst any) error {
	svc, ok := matchService(service)
	if !ok {
		return &UnknownServiceError{Service: service, Suggestion: closest(service, knownServiceNames())}
	}
	if strings.Contains(name, "__") {
		return &InvalidOptionsError{Service: svc, Reason: fmt.Sprintf("invalid instance name %q: must not contain %q", name, "__")}
	}
	prefix := strings.ToUpper(svc)
	named := name != ""
	if named {
		prefix += "__" + strings.ToUpper(name)
	}
	root := reflect.Indirect(reflect.ValueOf(dst))
	if !root.IsValid() || root.Kind() != reflect.Struct || !root.CanSet() {
		return &InvalidOptionsError{Service: svc, Reason: "dst must be a non-nil pointer to a struct"}
	}
	target := root
	setDst := dst
	var adapter reflect.Value
	var candidates []string
	envelope := false
	if opts, ok := findField(root, "options"); ok && opts.Kind() == reflect.Struct {
		if a, ok := findField(root, "adapter"); ok && a.Kind() == reflect.String && a.CanSet() {
			target = opts
			setDst = target.Addr().Interface()
			adapter = a
			envelope = true
			candidates = append([]string{"adapter"}, fieldCandidates(target)...)
		}
	}
	if candidates == nil {
		candidates = fieldCandidates(root)
	}
	for _, key := range prefixedEnvKeys(env, prefix, named) {
		rest, ok := applyPrefixKey(key, prefix, named)
		if !ok {
			continue
		}
		if envelope && normEqualFold(rest, "adapter") {
			adapter.SetString(env[key])
			continue
		}
		if !envelope && normEqualFold(rest, "adapter") {
			// Bare Options with no envelope: adapter selection is the
			// caller's job, so a bare ADAPTER key sets the field when
			// the struct has one and is otherwise ignored.
			if field, ok := findField(target, rest); ok && isScalarField(field) {
				if err := setOptionField(svc, setDst, []string{rest}, env[key], key); err != nil {
					return wrapApplyError(svc, err)
				}
			}
			continue
		}
		match := matchEnvField(target, rest)
		if match.Ambiguous {
			return &InvalidOptionsError{Service: svc, Reason: fmt.Sprintf("ambiguous env %s: matches fields %q and %q", key, strings.ToLower(rest), strings.ToLower(match.OuterName)+"."+strings.ToLower(match.InnerName))}
		}
		if match.Path != nil {
			if err := setOptionField(svc, setDst, match.Path, env[key], key); err != nil {
				return wrapApplyError(svc, err)
			}
			continue
		}
		unknown := unknownEnvFieldError(svc, rest, match.InnerName, match.HasNesting, match.OuterStruct, candidates)
		if unknown.Suggestion != "" || strictEnv(env) {
			return unknown
		}
		// Close to no known field: an unrelated ambient variable, not a
		// typo. Ignore it; it must not land on any field.
	}
	return nil
}

// envFieldMatch reports where one env field remainder lands inside an
// options struct.
type envFieldMatch struct {
	// Path holds the field path to set: one segment for a flat field, two
	// for a nested one. Nil when the remainder resolved to no settable
	// field.
	Path []string
	// OuterStruct is the nested struct the outer segment resolved to, if
	// any. It drives the suggestion for a nested remainder.
	OuterStruct reflect.Value
	// OuterName is the outer path segment of a nested remainder.
	OuterName string
	// InnerName is the inner path segment of a nested remainder.
	InnerName string
	// HasNesting reports that the remainder had a usable nesting split.
	HasNesting bool
	// Ambiguous reports that the remainder matched both a flat field and a
	// nested path: the key is a collision and always fails.
	Ambiguous bool
}

// matchEnvField matches a field remainder against the options struct
// target. A field that exists but holds no scalar value (a map, a slice, an
// interface) does not resolve: env values are strings, so such a field can
// never be set from the environment.
func matchEnvField(target reflect.Value, rest string) envFieldMatch {
	var m envFieldMatch
	flat, flatOK := findField(target, rest)
	outerName, innerName, hasNesting := cutEnvNesting(rest)
	var outerStruct reflect.Value
	var inner reflect.Value
	nestedOK := false
	if hasNesting {
		if outer, ok := findField(target, outerName); ok {
			if indir := reflect.Indirect(outer); indir.IsValid() && indir.Kind() == reflect.Struct {
				outerStruct = indir
				if in, ok := findField(indir, innerName); ok {
					inner = in
					nestedOK = true
				}
			}
		}
	}
	m.OuterStruct = outerStruct
	m.OuterName = outerName
	m.InnerName = innerName
	m.HasNesting = hasNesting
	switch {
	case flatOK && isScalarField(flat) && nestedOK && isScalarField(inner):
		m.Ambiguous = true
	case flatOK && isScalarField(flat):
		m.Path = []string{rest}
	case nestedOK:
		m.Path = []string{outerName, innerName}
	}
	return m
}

// applyPrefixKey reports the field remainder of an env key under the wired
// prefix. Default instances match PREFIX_FIELD (single underscore; double
// underscores belong to the instance namespace and are skipped). Named
// instances match PREFIX__FIELD. Everything else is not ours.
func applyPrefixKey(key, prefix string, named bool) (string, bool) {
	upper := strings.ToUpper(key)
	if named {
		sep := prefix + "__"
		if upper == prefix || !strings.HasPrefix(upper, sep) {
			return "", false
		}
		rest := upper[len(sep):]
		if rest == "" {
			return "", false
		}
		return rest, true
	}
	if upper == prefix || !strings.HasPrefix(upper, prefix+"_") {
		return "", false
	}
	rest := upper[len(prefix)+1:]
	if rest == "" || rest[0] == '_' {
		return "", false
	}
	return rest, true
}

// cutEnvNesting splits a remainder at its first underscore into outer and
// inner path segments. ok is false when there is no usable nesting.
func cutEnvNesting(rest string) (outer, inner string, ok bool) {
	idx := strings.IndexByte(rest, '_')
	if idx <= 0 || idx+1 >= len(rest) {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}

// isScalarField reports whether f accepts a string coercion via setScalar:
// durations, strings, bools, integers, unsigned integers, and floats.
func isScalarField(f reflect.Value) bool {
	if !f.IsValid() || !f.CanSet() {
		return false
	}
	if f.Type() == durationType {
		return true
	}
	switch f.Kind() { //nolint:exhaustive // default rejects every kind the env overlay doesn't support
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// wrapApplyError converts a setOptionField failure to the ApplyEnv contract:
// unknown fields surface as-is (they already carry a suggestion);
// unparseable values become InvalidOptionsError without echoing the value.
func wrapApplyError(svc string, err error) error {
	var unknown *UnknownFieldError
	if errors.As(err, &unknown) {
		return err
	}
	return &InvalidOptionsError{Service: svc, Reason: err.Error()}
}

// unknownEnvFieldError builds the UnknownFieldError for a remainder that
// resolved to no settable field, preferring the inner struct's candidates
// for the suggestion when the outer segment resolved. An empty Suggestion
// means nothing is close: the key is an ambient variable, not a typo.
func unknownEnvFieldError(svc, rest, innerName string, hasNesting bool, outerStruct reflect.Value, candidates []string) *UnknownFieldError {
	if hasNesting && outerStruct.IsValid() {
		if innerCands := fieldCandidates(outerStruct); len(innerCands) > 0 {
			return &UnknownFieldError{Service: svc, Field: rest, Suggestion: closest(innerName, innerCands)}
		}
	}
	return &UnknownFieldError{Service: svc, Field: rest, Suggestion: closest(rest, candidates)}
}

// InstanceNames lists the named instances configured for service in env by
// parsing SERVICE__NAME__* keys. Names normalize to uppercase and return
// sorted and deduplicated; malformed keys (empty name or field) are
// skipped. Unknown services and empty input yield nil.
func InstanceNames(env Env, service string) []string {
	svc, ok := matchService(service)
	if !ok {
		return nil
	}
	head := strings.ToUpper(svc) + "__"
	seen := make(map[string]bool)
	var out []string
	for _, key := range sortedEnvKeys(env) {
		upper := strings.ToUpper(key)
		if !strings.HasPrefix(upper, head) {
			continue
		}
		name, field, ok := strings.Cut(upper[len(head):], "__")
		if !ok || name == "" || field == "" {
			continue
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// DefaultOptions returns fresh default Options for service, applying the
// same zero-infrastructure defaults Default uses. name seeds named
// instances from their service default: every instance starts identical and
// diverges through ApplyEnv. Unknown services yield nil.
func DefaultOptions(service, name string) any {
	_ = name
	svc, ok := matchService(service)
	if !ok {
		return nil
	}
	_, opts, ok := serviceRefs(Default(), svc)
	if !ok {
		return nil
	}
	return reflect.Indirect(reflect.ValueOf(opts)).Interface()
}

// applyPluginEnv overlays `<PLUGIN>_ADAPTER` variables onto cfg.Plugins,
// one per existing plugin entry (uppercased). Missing or empty variables
// are ignored. Only the adapter is overridable; plugin options stay raw
// because they have no typed fields to coerce into.
func applyPluginEnv(env Env, cfg *Config) {
	for name, svc := range cfg.Plugins {
		v, ok := lookupEnvKey(env, strings.ToUpper(name)+"_ADAPTER")
		if !ok || v == "" {
			continue
		}
		svc.Adapter = v
		cfg.Plugins[name] = svc
	}
}

// matchService reports the canonical lowercase service name for an
// env-var head, matched case-insensitively without allocation. ok is
// false when head names no known service. Non-ASCII heads can never
// match the ASCII service names, so they reject on the first byte.
func matchService(head string) (string, bool) {
	if len(head) == 0 {
		return "", false
	}
	if !isASCII(head) {
		// Rare: a non-ASCII head whose ToLower form is ASCII (for
		// example the Kelvin sign folding to 'k') still cannot be
		// distinguished from its ASCII spelling, so fold the lowered
		// form; anything still non-ASCII rejects below.
		lower := strings.ToLower(head)
		if !isASCII(lower) {
			return "", false
		}
		head = lower
	}

	// Candidate names grouped by first letter: most env heads reject on
	// the first byte, so the fold-compare runs at most a few times.
	switch foldASCII(head[0]) {
	case 'a':
		return matchName(head, "ai", "analytics", "auth")
	case 'b':
		return matchName(head, "billing")
	case 'c':
		return matchName(head, "cache", "cdn", "crypto")
	case 'd':
		return matchName(head, "db", "document")
	case 'e':
		return matchName(head, "eventbus")
	case 'f':
		return matchName(head, "flag")
	case 'g':
		return matchName(head, "geo")
	case 'i':
		return matchName(head, "i18n", "idempotency")
	case 'l':
		return matchName(head, "lock", "log")
	case 'm':
		return matchName(head, "mailer", "media")
	case 'n':
		return matchName(head, "notification")
	case 'o':
		return matchName(head, "observability", "outbox")
	case 'p':
		return matchName(head, "password", "payment", "permission")
	case 'q':
		return matchName(head, "queue")
	case 'r':
		return matchName(head, "ratelimit", "resilience", "router")
	case 's':
		return matchName(head, "scheduler", "search", "secrets", "session", "storage")
	case 't':
		return matchName(head, "tenant")
	case 'v':
		return matchName(head, "vectorstore")
	case 'w':
		return matchName(head, "webhook", "workflow")
	}

	return "", false
}

// matchName reports the first of names that ASCII-fold-equals head.
func matchName(head string, names ...string) (string, bool) {
	for _, name := range names {
		if asciiEqualFold(head, name) {
			return name, true
		}
	}

	return "", false
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
	case "cdn":
		return &cfg.CDN.Adapter, &cfg.CDN.Options, true
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
	case "outbox":
		return &cfg.Outbox.Adapter, &cfg.Outbox.Options, true
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
	case "resilience":
		return &cfg.Resilience.Adapter, &cfg.Resilience.Options, true
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

// normEqualFold reports whether a and b are equal under normName
// normalization (case-insensitive, "_" ignored) without allocation.
// Non-ASCII inputs fall back to the allocating normName comparison.
func normEqualFold(a, b string) bool {
	if !isASCII(a) || !isASCII(b) {
		return normName(a) == normName(b)
	}

	i, j := 0, 0
	for i < len(a) || j < len(b) {
		for i < len(a) && a[i] == '_' {
			i++
		}
		for j < len(b) && b[j] == '_' {
			j++
		}
		if i >= len(a) || j >= len(b) {
			return i >= len(a) && j >= len(b)
		}
		if foldASCII(a[i]) != foldASCII(b[j]) {
			return false
		}
		i++
		j++
	}

	return true
}

// jsonFieldName reports the encoding/json key for a struct field: the tag
// name when present, otherwise the Go field name converted to snake_case.
func jsonFieldName(sf reflect.StructField) string {
	if tag, ok := sf.Tag.Lookup("json"); ok {
		if name, _, _ := strings.Cut(tag, ","); name != "" {
			return name
		}
	}
	// No json tag: convert Go field name (CamelCase) to snake_case.
	// Handles abbreviations (API -> api, DB -> db, ID -> id, JWT -> jwt, etc.)
	return camelToSnake(sf.Name)
}

// camelToSnake converts CamelCase to snake_case.
// Handles abbreviations: APIKey -> api_key, DBConn -> db_conn, ID -> id, JWTToken -> jwt_token, etc.
func camelToSnake(s string) string {
	if s == "" {
		return ""
	}
	var buf strings.Builder
	buf.Grow(len(s) * 2)
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				prev := s[i-1]
				next := byte(0)
				if i+1 < len(s) {
					next = s[i+1]
				}
				// Add underscore before uppercase if:
				// - previous char is lowercase (camelCase boundary)
				// - next char is lowercase (abbreviation boundary like APIKey -> api_key)
				if (prev >= 'a' && prev <= 'z') || (next >= 'a' && next <= 'z') {
					buf.WriteByte('_')
				}
			}
			buf.WriteRune(unicode.ToLower(r))
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
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
		if normEqualFold(jsonFieldName(sf), name) {
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
func setOptionField(service string, dst any, path []string, value string, envName ...string) error {
	field := strings.Join(path, ".")

	// envName names the source environment variable (e.g. DB_MAXCONNS) for
	// type-error text; direct callers and tests may omit it.
	name := ""
	if len(envName) > 0 {
		name = envName[0]
	}
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
	return setScalar(service, field, name, leaf, value)
}

// setScalar coerces the env string into f by kind. Durations parse via
// time.ParseDuration ("5s" style). Failures name the field, source env var,
// and expected kind via scrubbedValue — the raw value is never echoed, so
// secrets in env/file values cannot leak into error text.
func setScalar(service, field, envName string, f reflect.Value, value string) error {
	miss := func() error { return &UnknownFieldError{Service: service, Field: field} }
	if f.Type() == durationType {
		d, err := time.ParseDuration(value)
		if err != nil {
			return scrubbedValue(field, envName, "duration")
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
			return scrubbedValue(field, envName, f.Kind().String())
		}
		f.SetBool(b)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, f.Type().Bits())
		if err != nil {
			return scrubbedValue(field, envName, f.Kind().String())
		}
		f.SetInt(n)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, f.Type().Bits())
		if err != nil {
			return scrubbedValue(field, envName, f.Kind().String())
		}
		f.SetUint(n)
		return nil
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(value, f.Type().Bits())
		if err != nil {
			return scrubbedValue(field, envName, f.Kind().String())
		}
		f.SetFloat(n)
		return nil
	default:
		return miss()
	}
}

// scrubbedValue reports an unparseable env value by field name, source env
// var (when known), and expected kind only. strconv and time parse errors
// quote the offending input, which may be a secret, so only those survive.
func scrubbedValue(field, envName, expected string) error {
	if envName != "" {
		return fmt.Errorf("%w for field %s from env %s: expected %s", ErrInvalidValue, strconv.Quote(field), envName, expected)
	}

	return fmt.Errorf("%w for field %s: expected %s", ErrInvalidValue, strconv.Quote(field), expected)
}
