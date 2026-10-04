package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// addUsageBody is the long description for `zever add -h`.
var addUsageBody = `Adds one battery to the calling project: a require plus (for local
checkouts) a replace for the battery's core and adapter modules in go.mod,
a named import plus Register() call in internal/app/app.go, and a stanza
in zever.yaml with the default adapter.

The adapter may be omitted (` + "`zever add cache`" + ` selects
config.Default()'s own pick, memory) or pinned explicitly
(` + "`zever add cache/redis`" + ` or ` + "`zever add cache --adapter redis`" + `).
When both the slash form and --adapter are given they must agree, or the
command fails with a conflict error.

Re-adding an already-present battery without --force is an error naming
the current adapter; pass --force to overwrite the zever.yaml stanza
(go.mod/app.go edits stay append-only, skipped when already present).

Third-party plugin batteries (any name outside config.Default()'s own
set) resolve to the plugin path with a stderr warning: a ` + "`plugins:`" + `
stanza in zever.yaml, a require of the plugin module in go.mod, and a
RegisterPlugin/Resolve wiring comment in internal/app/app.go. Plugins
need an explicit adapter (` + "`zever add sms/stub --module ...`" + `) and
--module <path[@version]> naming the plugin's Go module (version
defaults to v0.1.0 when the @version suffix is omitted).

WHAT THIS DOES NOT DO: it does not run ` + "`go mod tidy`" + ` (run it
afterwards so go.sum catches up) and it never overwrites hand-written code
-- every edit is an append, skipped when already present. Run it from your
project root (the directory holding go.mod).`

// newAddFlagSet builds the stdlib flag set for `zever add`: the single
// source for runAdd parsing and for help printers, so help can never
// drift from the real flags (mirrors newNewFlagSet).
func newAddFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.String("adapter", "", "adapter pick, overriding the positional battery/adapter slash form when both agree")
	fs.Bool("force", false, "overwrite the zever.yaml stanza when the battery is already present")
	fs.String("module", "", "plugin battery's Go module path, with optional @version suffix (required for plugin batteries)")

	fs.Usage = func() {
		_, _ = fmt.Fprintln(fs.Output(), addUsageBody)
		fs.PrintDefaults()
	}

	return fs
}

// splitModuleVersion splits a --module value into its module path and
// version. "path@version" pins version explicitly; a bare path defaults
// to v0.1.0 (third-party plugins version on their own schedule, so the
// framework pin never applies to them).
func splitModuleVersion(raw string) (path, version string) {
	if i := strings.LastIndex(raw, "@"); i != -1 && i+1 < len(raw) {
		return raw[:i], raw[i+1:]
	}

	return raw, "v0.1.0"
}

// resolveAddTarget splits a `zever add` positional into its battery and
// effective adapter, honouring --adapter. A slash adapter and a different
// --adapter value is a conflict error. Known batteries resolve against
// allServiceAdapters (bare names take config.Default()'s own pick, and
// unknown adapters for a known battery fail closed); unknown names fall
// through to the plugin path, which needs an explicit adapter.
func resolveAddTarget(arg, flagAdapter string) (battery, adapter string, isPlugin bool, err error) {
	const tag = "zever add"

	if arg == "" {
		return "", "", false, fmt.Errorf("%s: expected <battery>[/<adapter>], got an empty argument", tag)
	}

	if strings.Count(arg, "/") > 1 {
		return "", "", false, fmt.Errorf("%s: %q wants <battery>[/<adapter>] (a single optional /)", tag, arg)
	}

	battery, slashAdapter, _ := strings.Cut(arg, "/")

	if battery == "" || (strings.Contains(arg, "/") && slashAdapter == "") {
		return "", "", false, fmt.Errorf("%s: %q wants <battery>[/<adapter>] with no empty part", tag, arg)
	}

	if flagAdapter != "" && slashAdapter != "" && flagAdapter != slashAdapter {
		return "", "", false, fmt.Errorf("%s: adapter conflict: positional %q pins %q but --adapter selects %q (drop one or make them agree)", tag, arg, slashAdapter, flagAdapter)
	}

	adapter = slashAdapter
	if flagAdapter != "" {
		adapter = flagAdapter
	}

	if _, ok := allServiceAdapters[battery]; ok {
		if adapter == "" {
			adapter = defaultServiceAdapter(battery)
		}

		choices := adaptersForBattery(battery)

		found := false

		for _, c := range choices {
			if c == adapter {
				found = true
				break
			}
		}

		if !found {
			return "", "", false, fmt.Errorf("%s: unknown adapter %q for battery %q (choices: %s)", tag, adapter, battery, strings.Join(choices, ", "))
		}

		return battery, adapter, false, nil
	}

	if !isValidAppName(battery) {
		return "", "", true, fmt.Errorf("%s: %q is not a valid plugin battery name (letters, digits, - and _ only)", tag, battery)
	}

	if adapter == "" {
		return "", "", true, fmt.Errorf("%s: plugin battery %q needs an explicit adapter (use %q/<adapter> or --adapter)", tag, battery, battery)
	}

	return battery, adapter, true, nil
}

// parseAddArg splits a `zever add` positional into its battery and adapter.
// "cache" resolves the adapter from config.Default()'s own pick (see
// defaultServiceAdapter); "cache/redis" pins redis explicitly. Anything
// else -- empty halves, extra slashes, unknown batteries -- is an error.
func parseAddArg(arg string) (battery, adapter string, err error) {
	const tag = "zever add"

	if arg == "" {
		return "", "", fmt.Errorf("%s: expected <battery>[/<adapter>], got an empty argument", tag)
	}

	if strings.Count(arg, "/") > 1 {
		return "", "", fmt.Errorf("%s: %q wants <battery>[/<adapter>] (a single optional /)", tag, arg)
	}

	battery, adapter, _ = strings.Cut(arg, "/")

	if battery == "" || (strings.Contains(arg, "/") && adapter == "") {
		return "", "", fmt.Errorf("%s: %q wants <battery>[/<adapter>] with no empty part", tag, arg)
	}

	if _, ok := allServiceAdapters[battery]; !ok {
		return "", "", fmt.Errorf("%s: unknown battery %q (want one of: %s)", tag, battery, strings.Join(sortedKeys(allServiceAdapters), ", "))
	}

	if adapter == "" {
		adapter = defaultServiceAdapter(battery)
	}

	return battery, adapter, nil
}

// goModRequireVersion scans go.mod source for the version pinned by the
// require of modulePath, handling both single-line requires and require
// blocks. It returns "" when no require of that path exists.
func goModRequireVersion(src, modulePath string) string {
	inBlock := false

	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "require ("):
			inBlock = true

			continue
		case inBlock && trimmed == ")":
			inBlock = false

			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}

		if !inBlock {
			if len(fields) < 3 || fields[0] != "require" {
				continue
			}

			fields = fields[1:]
		}

		if fields[0] == modulePath {
			return strings.Trim(fields[1], `"`)
		}
	}

	return ""
}

// goModHasRequire reports whether go.mod source already requires modulePath
// (any version, either require form).
func goModHasRequire(src, modulePath string) bool {
	return goModRequireVersion(src, modulePath) != ""
}

// goModRootReplaceTarget scans go.mod source for a single-line
// `replace github.com/zenta-dev/zever => <target>` and returns the target
// (unquoted), or "" when there is none. Only the framework root replace
// is read: it is the marker of a local-checkout project, and the base
// every derived adapter replace is computed from.
func goModRootReplaceTarget(src string) string {
	for _, line := range strings.Split(src, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 4 && fields[0] == "replace" && fields[1] == frameworkModulePath && fields[2] == "=>" {
			return strings.Trim(fields[3], `"`)
		}
	}

	return ""
}

// isLocalReplaceTarget reports whether a replace target names a local
// directory (rather than a module version): derived adapter replaces are
// only emitted for those.
func isLocalReplaceTarget(target string) bool {
	return strings.HasPrefix(target, ".") || strings.HasPrefix(target, "/")
}

// updateGoModForAdd ensures go.mod source requires the battery's core and
// adapter modules at version, adding local-checkout replaces derived from
// the framework root replace when the caller resolves against one. Paths
// already required (any version) or already replaced are left untouched.
func updateGoModForAdd(src string, sel batterySelection, version string) string {
	paths := []string{coreModulePath(sel.Battery), adapterModulePath(sel)}

	lines := strings.Split(src, "\n")

	// Insert new requires after the last single-line require, or inside
	// the first require block, or at the end when there are none.
	blockClose := -1
	lastSingle := -1

	inBlock := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "require ("):
			inBlock = true
		case inBlock && trimmed == ")":
			if blockClose == -1 {
				blockClose = i
			}

			inBlock = false
		case !inBlock && strings.HasPrefix(trimmed, "require "):
			lastSingle = i
		}
	}

	var missing []string

	for _, path := range paths {
		if !goModHasRequire(strings.Join(lines, "\n"), path) {
			missing = append(missing, path+" "+version)
		}
	}

	// Block entries are bare indented lines; single-line requires carry
	// the require keyword with no indent.
	for i, m := range missing {
		if blockClose != -1 {
			missing[i] = "\t" + m
		} else {
			missing[i] = "require " + m
		}
	}

	switch {
	case blockClose != -1:
		lines = append(lines[:blockClose], append(missing, lines[blockClose:]...)...)
	case lastSingle != -1:
		lines = append(lines[:lastSingle+1], append(missing, lines[lastSingle+1:]...)...)
	default:
		lines = append(lines, missing...)
	}

	out := strings.Join(lines, "\n")

	if target := goModRootReplaceTarget(out); isLocalReplaceTarget(target) {
		var extra []string

		for _, path := range paths {
			replace := "replace " + path + " => "
			if strings.Contains(out, replace) {
				continue
			}

			rel := filepath.ToSlash(filepath.Join(target, strings.TrimPrefix(path, frameworkModulePath+"/")))
			extra = append(extra, replace+rel)
		}

		if len(extra) > 0 {
			if !strings.HasSuffix(out, "\n") {
				out += "\n"
			}

			out += strings.Join(extra, "\n") + "\n"
		}
	}

	return out
}

// updateAppGoForAdd splices one battery selection's named import and
// Register() call into an existing internal/app/app.go: the import goes
// before the import block's closing paren, the call right after New's
// opening line. Either half already present is skipped, so re-running add
// for the same battery is a no-op. The result is gofmt-normalized.
func updateAppGoForAdd(src string, sel batterySelection) (string, error) {
	const tag = "zever add"

	alias, path, call := adapterBinding(sel)

	if !strings.Contains(src, `"`+path+`"`) {
		start := strings.Index(src, "import (")
		if start == -1 {
			return "", fmt.Errorf("%s: internal/app/app.go has no import block to extend", tag)
		}

		rest := src[start:]
		closeIdx := strings.Index(rest, "\n)")
		if closeIdx == -1 {
			return "", fmt.Errorf("%s: internal/app/app.go has an unterminated import block", tag)
		}

		src = src[:start+closeIdx] + "\n\t" + alias + ` "` + path + `"` + src[start+closeIdx:]
	}

	if !strings.Contains(src, call) {
		idx := strings.Index(src, "func New()")
		if idx == -1 {
			return "", fmt.Errorf("%s: internal/app/app.go has no New to wire the Register call into", tag)
		}

		eol := strings.Index(src[idx:], "\n")
		if eol == -1 {
			return "", fmt.Errorf("%s: internal/app/app.go has a truncated New declaration", tag)
		}

		at := idx + eol + 1
		src = src[:at] + "\t" + call + "\n" + src[at:]
	}

	formatted, err := format.Source([]byte(src))
	if err != nil {
		return "", fmt.Errorf("%s: updated internal/app/app.go is not valid Go: %w", tag, err)
	}

	return string(formatted), nil
}

// updateZeverYamlForAdd sets batteries' adapter pick in zever.yaml source,
// preserving every other entry (including the entry's own options). A
// missing or empty file starts a fresh document.
func updateZeverYamlForAdd(src string, sel batterySelection) ([]byte, error) {
	doc := map[string]any{}

	if strings.TrimSpace(src) != "" {
		if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
			return nil, fmt.Errorf("zever add: parse zever.yaml: %w", err)
		}

		if doc == nil {
			doc = map[string]any{}
		}
	}

	if existing, ok := doc[sel.Battery].(map[string]any); ok {
		existing["adapter"] = sel.Adapter
	} else {
		doc[sel.Battery] = map[string]any{"adapter": sel.Adapter}
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("zever add: marshal zever.yaml: %w", err)
	}

	return data, nil
}

// currentCoreAdapter reads the adapter already pinned for battery in
// zever.yaml source, or "" when the stanza is absent.
func currentCoreAdapter(src, battery string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}

	doc := map[string]any{}

	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return ""
	}

	if existing, ok := doc[battery].(map[string]any); ok {
		if a, ok := existing["adapter"].(string); ok {
			return a
		}
	}

	return ""
}

// currentPluginAdapter reads the adapter already pinned for a plugin
// battery under the top-level `plugins:` block, or "" when absent.
func currentPluginAdapter(src, plugin string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}

	doc := map[string]any{}

	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return ""
	}

	plugins, ok := doc["plugins"].(map[string]any)
	if !ok {
		return ""
	}

	if existing, ok := plugins[plugin].(map[string]any); ok {
		if a, ok := existing["adapter"].(string); ok {
			return a
		}
	}

	return ""
}

// updateZeverYamlForPlugin sets a plugin battery's adapter pick under the
// top-level `plugins:` block (the direct-map shape config decode accepts),
// preserving every other entry. A missing or empty file starts fresh.
func updateZeverYamlForPlugin(src, plugin, adapter string) ([]byte, error) {
	doc := map[string]any{}

	if strings.TrimSpace(src) != "" {
		if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
			return nil, fmt.Errorf("zever add: parse zever.yaml: %w", err)
		}

		if doc == nil {
			doc = map[string]any{}
		}
	}

	plugins, ok := doc["plugins"].(map[string]any)
	if !ok || plugins == nil {
		plugins = map[string]any{}
	}

	if existing, ok := plugins[plugin].(map[string]any); ok {
		existing["adapter"] = adapter
	} else {
		plugins[plugin] = map[string]any{"adapter": adapter}
	}

	doc["plugins"] = plugins

	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("zever add: marshal zever.yaml: %w", err)
	}

	return data, nil
}

// updateGoModForPlugin ensures go.mod source requires the plugin module at
// version, inserted after the last single-line require, inside the first
// require block, or at the end when there are none. Already-required paths
// (any version) are left untouched. Third-party modules never get local
// replaces.
func updateGoModForPlugin(src, modulePath, version string) string {
	if goModHasRequire(src, modulePath) {
		return src
	}

	lines := strings.Split(src, "\n")

	blockClose := -1
	lastSingle := -1

	inBlock := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "require ("):
			inBlock = true
		case inBlock && trimmed == ")":
			if blockClose == -1 {
				blockClose = i
			}

			inBlock = false
		case !inBlock && strings.HasPrefix(trimmed, "require "):
			lastSingle = i
		}
	}

	var missing string
	if blockClose != -1 {
		missing = "\t" + modulePath + " " + version
	} else {
		missing = "require " + modulePath + " " + version
	}

	switch {
	case blockClose != -1:
		lines = append(lines[:blockClose], append([]string{missing}, lines[blockClose:]...)...)
	case lastSingle != -1:
		lines = append(lines[:lastSingle+1], append([]string{missing}, lines[lastSingle+1:]...)...)
	default:
		lines = append(lines, missing)
	}

	return strings.Join(lines, "\n")
}

// pluginImportAlias maps a plugin battery name to a valid Go import alias
// (dashes become underscores; leading digits gain a plugin prefix).
func pluginImportAlias(plugin string) string {
	alias := strings.ReplaceAll(plugin, "-", "_")
	if alias == "" {
		return "plugin"
	}

	if alias[0] >= '0' && alias[0] <= '9' {
		return "plugin" + alias
	}

	return alias
}

// updateAppGoForPlugin splices a plugin battery's wiring guidance into an
// existing internal/app/app.go at the same points updateAppGoForAdd uses:
// a blank import of the plugin module before the import block's closing
// paren (compiles without use, pulling the module into the build), and a
// comment block right after New's opening line showing the
// RegisterPlugin/Resolve calls to fill in. Either half already present is
// skipped, so re-running is a no-op. Comments keep the file compiling
// before the developer fills in the plugin's concrete types. The result
// is gofmt-normalized.
func updateAppGoForPlugin(src, plugin, modulePath string) (string, error) {
	const tag = "zever add"

	alias := pluginImportAlias(plugin)

	if !strings.Contains(src, `"`+modulePath+`"`) {
		start := strings.Index(src, "import (")
		if start == -1 {
			return "", fmt.Errorf("%s: internal/app/app.go has no import block to extend", tag)
		}

		rest := src[start:]
		closeIdx := strings.Index(rest, "\n)")
		if closeIdx == -1 {
			return "", fmt.Errorf("%s: internal/app/app.go has an unterminated import block", tag)
		}

		src = src[:start+closeIdx] + "\n\t_ \"" + modulePath + "\" // plugin " + plugin + " (" + alias + ")" + src[start+closeIdx:]
	}

	marker := `Plugin "` + plugin + `"`

	if !strings.Contains(src, marker) {
		idx := strings.Index(src, "func New()")
		if idx == -1 {
			return "", fmt.Errorf("%s: internal/app/app.go has no New to wire the Register call into", tag)
		}

		eol := strings.Index(src[idx:], "\n")
		if eol == -1 {
			return "", fmt.Errorf("%s: internal/app/app.go has a truncated New declaration", tag)
		}

		at := idx + eol + 1
		block := "\t// " + marker + " (third-party battery, see zever.yaml plugins." + plugin + "):\n" +
			"\t// wire it here after `go mod tidy`, e.g.:\n" +
			"\t//   if err := container.RegisterPlugin(\"" + plugin + "\", container.PluginAPIVersion, ...); err != nil { return nil, err }\n" +
			"\t//   svc, err := container.Resolve(c, \"" + plugin + "\")\n"
		src = src[:at] + block + src[at:]
	}

	formatted, err := format.Source([]byte(src))
	if err != nil {
		return "", fmt.Errorf("%s: updated internal/app/app.go is not valid Go: %w", tag, err)
	}

	return string(formatted), nil
}

// addBatteryToProject performs the three `zever add` edits against the
// calling project in the working directory: go.mod requires (+ local
// replaces), internal/app/app.go import + Register(), and the zever.yaml
// stanza. It runs no network commands: `go mod tidy` afterwards is the
// caller's to run so go.sum catches up.
func addBatteryToProject(tag, battery, adapter string, force bool) error {
	sel := batterySelection{Battery: battery, Adapter: adapter}

	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no go.mod in the working directory (run this from your project root)", tag)
		}

		return fmt.Errorf("%s: read go.mod: %w", tag, err)
	}

	yamlProbe, err := os.ReadFile("zever.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: read zever.yaml: %w", tag, err)
	}

	if current := currentCoreAdapter(string(yamlProbe), battery); current != "" && !force {
		return fmt.Errorf("%s: battery %q already added with adapter %q (pass --force to overwrite)", tag, battery, current)
	}

	version := goModRequireVersion(string(gomod), frameworkModulePath)
	if version == "" {
		version = FrameworkVersion
	}

	updated := updateGoModForAdd(string(gomod), sel, version)
	if updated != string(gomod) {
		if werr := os.WriteFile("go.mod", []byte(updated), 0o644); werr != nil { //nolint:gosec // go.mod is not a secret
			return fmt.Errorf("%s: write go.mod: %w", tag, werr)
		}
	}

	appPath := filepath.Join("internal", "app", "app.go")

	appSrc, err := os.ReadFile(appPath) //nolint:gosec // project-owned scaffold path
	if err != nil {
		return fmt.Errorf("%s: read %q: %w (scaffold it with `zever generate server` first)", tag, appPath, err)
	}

	updatedApp, err := updateAppGoForAdd(string(appSrc), sel)
	if err != nil {
		return err
	}

	if updatedApp != string(appSrc) {
		if werr := os.WriteFile(appPath, []byte(updatedApp), 0o644); werr != nil { //nolint:gosec // project-owned scaffold path
			return fmt.Errorf("%s: write %q: %w", tag, appPath, werr)
		}
	}

	yamlSrc, err := os.ReadFile("zever.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: read zever.yaml: %w", tag, err)
	}

	updatedYaml, err := updateZeverYamlForAdd(string(yamlSrc), sel)
	if err != nil {
		return err
	}

	// A brand-new zever.yaml gets the schema modeline, like `zever new`
	// writes; an existing file is left header-untouched.
	if len(yamlSrc) == 0 {
		updatedYaml = append([]byte(zeverYamlSchemaModeline), updatedYaml...)
	}

	if err := os.WriteFile("zever.yaml", updatedYaml, 0o644); err != nil { //nolint:gosec // project config, not a secret
		return fmt.Errorf("%s: write zever.yaml: %w", tag, err)
	}

	return nil
}

// addPluginToProject performs the three `zever add` edits for a
// third-party plugin battery: a require of the plugin module in go.mod,
// the RegisterPlugin/Resolve wiring comment in internal/app/app.go, and
// the `plugins:` stanza in zever.yaml. Like the core path it runs no
// network commands.
func addPluginToProject(tag, plugin, adapter, modulePath, version string, force bool) error {
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no go.mod in the working directory (run this from your project root)", tag)
		}

		return fmt.Errorf("%s: read go.mod: %w", tag, err)
	}

	yamlProbe, err := os.ReadFile("zever.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: read zever.yaml: %w", tag, err)
	}

	if current := currentPluginAdapter(string(yamlProbe), plugin); current != "" && !force {
		return fmt.Errorf("%s: plugin battery %q already added with adapter %q (pass --force to overwrite)", tag, plugin, current)
	}

	updated := updateGoModForPlugin(string(gomod), modulePath, version)
	if updated != string(gomod) {
		if werr := os.WriteFile("go.mod", []byte(updated), 0o644); werr != nil { //nolint:gosec // go.mod is not a secret
			return fmt.Errorf("%s: write go.mod: %w", tag, werr)
		}
	}

	appPath := filepath.Join("internal", "app", "app.go")

	appSrc, err := os.ReadFile(appPath) //nolint:gosec // project-owned scaffold path
	if err != nil {
		return fmt.Errorf("%s: read %q: %w (scaffold it with `zever generate server` first)", tag, appPath, err)
	}

	updatedApp, err := updateAppGoForPlugin(string(appSrc), plugin, modulePath)
	if err != nil {
		return err
	}

	if updatedApp != string(appSrc) {
		if werr := os.WriteFile(appPath, []byte(updatedApp), 0o644); werr != nil { //nolint:gosec // project-owned scaffold path
			return fmt.Errorf("%s: write %q: %w", tag, appPath, werr)
		}
	}

	yamlSrc, err := os.ReadFile("zever.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: read zever.yaml: %w", tag, err)
	}

	updatedYaml, err := updateZeverYamlForPlugin(string(yamlSrc), plugin, adapter)
	if err != nil {
		return err
	}

	if len(yamlSrc) == 0 {
		updatedYaml = append([]byte(zeverYamlSchemaModeline), updatedYaml...)
	}

	if err := os.WriteFile("zever.yaml", updatedYaml, 0o644); err != nil { //nolint:gosec // project config, not a secret
		return fmt.Errorf("%s: write zever.yaml: %w", tag, err)
	}

	return nil
}

// runAdd implements `zever add <battery>[/<adapter>] [--adapter <name>]
// [--force] [--module <path[@version]>]`: one positional, no prompting.
// Guided input (if ever needed) belongs in the TUI layer; this stays the
// non-interactive entry point.
func runAdd(args []string) error {
	const tag = "zever add"

	fs := newAddFlagSet()

	positional, err := flexibleParse(fs, args)
	if err != nil {
		return err
	}

	flagString := func(name string) string {
		if f := fs.Lookup(name); f != nil {
			return f.Value.String()
		}

		return ""
	}

	if len(positional) != 1 {
		return fmt.Errorf("%s: expected exactly one <battery>[/<adapter>], got %d\n%s", tag, len(positional), addUsageBody)
	}

	battery, adapter, isPlugin, err := resolveAddTarget(positional[0], flagString("adapter"))
	if err != nil {
		return err
	}

	force := flagBool(fs, "force")

	if isPlugin {
		rawModule := flagString("module")
		if rawModule == "" {
			return fmt.Errorf("%s: plugin battery %q needs --module <path[@version]> naming its Go module", tag, battery)
		}

		modulePath, version := splitModuleVersion(rawModule)
		if modulePath == "" {
			return fmt.Errorf("%s: --module wants <path[@version]> with no empty path", tag)
		}

		_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("battery %q is not built in; wiring it as a third-party plugin via %q", battery, modulePath)))

		if err := addPluginToProject(tag, battery, adapter, modulePath, version, force); err != nil {
			return err
		}
	} else {
		if flagString("module") != "" {
			return fmt.Errorf("%s: --module only applies to plugin batteries (battery %q is built in)", tag, battery)
		}

		if err := addBatteryToProject(tag, battery, adapter, force); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintln(os.Stdout, success("✔ added ")+bold(battery)+dim(" with adapter ")+cyan(adapter))

	next := joinLines(
		bold("Next steps:"),
		"  "+dim("1.")+"  "+cmd("go mod tidy"),
		"  "+dim("2.")+"  "+cmd("zever serve"),
	)

	if colorEnabled {
		_, _ = fmt.Fprintln(os.Stdout, box(next))
	} else {
		_, _ = fmt.Fprintln(os.Stdout, next)
	}

	if shouldShowHint() {
		_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("battery %q is now pinned in internal/app/app.go and zever.yaml", battery)))
	}

	return nil
}
