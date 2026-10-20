package main

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
)

// scaffoldSelections is the selection list `zever new` builds for every
// battery the scaffolder knows: the whole service universe, so a key that
// only one battery carries is covered too.
func scaffoldSelections() []batterySelection {
	sel := make([]batterySelection, 0, len(allServiceAdapters))

	for name, adapter := range allServiceAdapters {
		sel = append(sel, batterySelection{Battery: name, Adapter: adapter})
	}

	return sel
}

// envFileKeys parses a rendered env file into its keys and values, the way
// config's dotenv parser reads it: comments and blank lines are skipped.
func envFileKeys(t *testing.T, src string) (config.Env, []string) {
	t.Helper()

	env := config.Env{}

	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("env line %q has no KEY=VALUE", line)
		}

		env[key] = value
	}

	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return env, keys
}

// serviceOptionsPtr returns a fresh pointer to the service's Options
// struct, seeded with the service defaults: ApplyEnv needs a settable
// pointer, DefaultOptions returns a value.
func serviceOptionsPtr(t *testing.T, service string) any {
	t.Helper()

	seed := config.DefaultOptions(service, "")
	if seed == nil {
		t.Fatalf("config.DefaultOptions(%q) = nil, want the service defaults", service)
	}

	ptr := reflect.New(reflect.TypeOf(seed))
	ptr.Elem().Set(reflect.ValueOf(seed))

	return ptr.Interface()
}

// TestScaffoldedEnvNamesTheDatabase pins the one env value the scaffold
// writes itself: the sqlite path. The CLI (`zever db migrate --dsn=...`),
// the server (DB_PATH) and the README must name the SAME file -- a project
// that migrates one database and serves another fails at its first query.
func TestScaffoldedEnvNamesTheDatabase(t *testing.T) {
	t.Parallel()

	const want = "DB_PATH=data/app.db"

	example := string(renderEnvExample(scaffoldSelections()))
	if !strings.Contains(example, want) {
		t.Errorf(".env.example lacks %q", want)
	}

	dev, err := renderDevEnv(scaffoldSelections())
	if err != nil {
		t.Fatalf("renderDevEnv: %v", err)
	}

	if !strings.Contains(string(dev), want) {
		t.Errorf(".env lacks %q", want)
	}
}

// TestScaffoldedEnvKeysResolve pins the .env.example contract: every key
// the scaffolder writes names a real option, so a fresh project resolves
// its config. The keys are the options' serialized names (their json
// tags), because that is what the resolver matches: a key spelled after
// the Go field name (PASSWORD_SALTLEN for salt_length) resolves to
// nothing and takes the whole config down with it.
//
// The check walks the same battery list the scaffolder uses and resolves
// every key on its own, so a failure names the key that broke.
func TestScaffoldedEnvKeysResolve(t *testing.T) {
	t.Parallel()

	env, keys := envFileKeys(t, string(renderEnvExample(scaffoldSelections())))

	if len(keys) == 0 {
		t.Fatal("the rendered .env.example has no keys")
	}

	for _, key := range keys {
		service, _, _ := strings.Cut(key, "_")
		if service == "" {
			t.Errorf("key %q has no service prefix", key)

			continue
		}

		if config.DefaultOptions(strings.ToLower(service), "") == nil {
			t.Errorf("key %q names no known service", key)

			continue
		}

		// Strict mode rejects every unknown key, so an unknown field
		// surfaces instead of being ignored as an ambient variable.
		one := config.Env{config.StrictEnvKey: "1", key: env[key]}

		err := config.ApplyEnv(one, strings.ToLower(service), "", serviceOptionsPtr(t, service))
		if err == nil {
			continue
		}

		var unknown *config.UnknownFieldError
		if errors.As(err, &unknown) {
			t.Errorf("key %q names no option: %v", key, err)

			continue
		}

		t.Errorf("key %q does not apply: %v", key, err)
	}
}

// TestScaffoldedEnvExampleResolves pins the whole file: copying it to
// .env must resolve the config, values included. A key that names no
// option is only half the contract -- a value the resolver cannot parse
// (or an adapter selector left empty) breaks the boot just as hard.
func TestScaffoldedEnvExampleResolves(t *testing.T) {
	t.Parallel()

	env, _ := envFileKeys(t, string(renderEnvExample(scaffoldSelections())))

	if _, err := config.Resolve(env); err != nil {
		t.Fatalf("the generated .env.example does not resolve: %v", err)
	}
}

// TestScaffoldedEnvExampleResolvesInStrictMode pins the strict contract:
// with ZEVER_CONFIG_STRICT=1 every generated key must still be a known
// option. Strict mode is the fail-closed switch, so a key that only
// resolves because unknown keys are ignored is still a broken key.
func TestScaffoldedEnvExampleResolvesInStrictMode(t *testing.T) {
	t.Parallel()

	env, _ := envFileKeys(t, string(renderEnvExample(scaffoldSelections())))
	env[config.StrictEnvKey] = "1"

	if _, err := config.Resolve(env); err != nil {
		t.Fatalf("the generated .env.example does not resolve in strict mode: %v", err)
	}
}

// TestScaffoldedDevEnvKeysResolve pins the generated .env (the dev secrets
// file): its keys name real options too, so the dev file resolves as it
// stands.
func TestScaffoldedDevEnvKeysResolve(t *testing.T) {
	t.Parallel()

	src, err := renderDevEnv(scaffoldSelections())
	if err != nil {
		t.Fatalf("renderDevEnv: %v", err)
	}

	env, keys := envFileKeys(t, string(src))

	if len(keys) == 0 {
		t.Fatal("the rendered .env has no keys")
	}

	for _, key := range keys {
		service, _, _ := strings.Cut(key, "_")
		if config.DefaultOptions(strings.ToLower(service), "") == nil {
			t.Errorf("key %q names no known service", key)

			continue
		}

		one := config.Env{config.StrictEnvKey: "1", key: env[key]}

		err := config.ApplyEnv(one, strings.ToLower(service), "", serviceOptionsPtr(t, service))
		if err == nil {
			continue
		}

		var unknown *config.UnknownFieldError
		if errors.As(err, &unknown) {
			t.Errorf("key %q names no option: %v", key, err)

			continue
		}

		t.Errorf("key %q does not apply: %v", key, err)
	}
}

// TestScaffoldedNamedInstanceEnvKeysResolve pins the named-instance form
// `zever add --name` writes: <SERVICE>__<NAME>__<FIELD> reaches the same
// option as the default instance's <SERVICE>_<FIELD>.
func TestScaffoldedNamedInstanceEnvKeysResolve(t *testing.T) {
	t.Parallel()

	const name = "reporting"

	sel := batterySelection{Battery: "db", Adapter: "sqlite"}
	section := envExampleSection(sel, name)

	env, keys := envFileKeys(t, section)

	if len(keys) == 0 {
		t.Fatal("the rendered section has no keys")
	}

	for _, key := range keys {
		one := config.Env{config.StrictEnvKey: "1", key: env[key]}

		err := config.ApplyEnv(one, "db", name, serviceOptionsPtr(t, "db"))
		if err == nil {
			continue
		}

		var unknown *config.UnknownFieldError
		if errors.As(err, &unknown) {
			t.Errorf("key %q names no option: %v", key, err)

			continue
		}

		t.Errorf("key %q does not apply: %v", key, err)
	}
}

// TestEnvKeyNameUsesTheSerializedName pins the naming rule: a key segment
// is the option's serialized name (its json tag), upper-cased. The Go
// field name is not a substitute -- the resolver ignores only case and
// "_", so SaltLen never matches salt_length.
func TestEnvKeyNameUsesTheSerializedName(t *testing.T) {
	t.Parallel()

	type options struct {
		SaltLen string `json:"salt_length"`
		Plain   string
		Omit    string `json:",omitempty"`
		Hidden  string `json:"-"`
	}

	tests := []struct {
		field string
		want  string
	}{
		{field: "SaltLen", want: "SALT_LENGTH"},
		{field: "Plain", want: "PLAIN"},
		{field: "Omit", want: "OMIT"},
		{field: "Hidden", want: "HIDDEN"},
	}

	tp := reflect.TypeOf(options{})

	for _, tc := range tests {
		sf, ok := tp.FieldByName(tc.field)
		if !ok {
			t.Fatalf("field %q not found", tc.field)
		}

		if got := envKeyName(sf); got != tc.want {
			t.Errorf("envKeyName(%s) = %q, want %q", tc.field, got, tc.want)
		}
	}
}

// TestEnvKeyNameSkipsHiddenFields pins the json:"-" rule: a field that is
// not on the wire has no env key at all, so the generator skips it
// instead of writing a key the resolver can never match. The tag is built
// at runtime: a literal one in the source trips the struct-tag linter.
func TestEnvKeyNameSkipsHiddenFields(t *testing.T) {
	t.Parallel()

	type options struct {
		Hidden  string `json:"-"`
		Visible string `json:"visible"`
	}

	tp := reflect.TypeOf(options{})

	hidden, ok := tp.FieldByName("Hidden")
	if !ok {
		t.Fatal("field Hidden not found")
	}

	if !jsonHidden(hidden) {
		t.Error("jsonHidden(Hidden) = false, want true")
	}

	visible, ok := tp.FieldByName("Visible")
	if !ok {
		t.Fatal("field Visible not found")
	}

	if jsonHidden(visible) {
		t.Error("jsonHidden(Visible) = true, want false")
	}

	// The same field with options after the name is hidden too.
	withOpts := reflect.StructOf([]reflect.StructField{{
		Name: "Hidden",
		Type: reflect.TypeOf(""),
		Tag:  reflect.StructTag(`json:"` + "-" + `,omitempty"`),
	}})

	sf, found := withOpts.FieldByName("Hidden")
	if !found {
		t.Fatal("field Hidden not found")
	}

	if !jsonHidden(sf) {
		t.Error("jsonHidden(hidden with options) = false, want true")
	}
}
