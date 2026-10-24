package session

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// referenceDoc is the per-battery reference page generated from this package.
// The tests below pin it to the code: every Options key, and every
// sentinel error must appear on the page. Edit the page and the code together.
const referenceDoc = "../../docs/src/content/docs/reference/session.mdx"

// TestReferencePageListsOptionKeys pins the reference options table to the
// Options struct: every JSON key (recursing into named blocks, skipping the
// anonymous shared-redis embed) must appear on the page.
func TestReferencePageListsOptionKeys(t *testing.T) {
	t.Parallel()

	raw := mustReadReference(t)
	for _, key := range optionKeys(t, reflect.TypeOf(Options{}), "") {
		if !strings.Contains(raw, "`"+key+"`") {
			t.Errorf("option key %q missing from reference page", key)
		}
	}
}

// TestReferencePageListsErrors pins the reference errors index to errors.go:
// every package-level ErrXxx sentinel must appear on the page with its cause
// and fix. Add a row when adding a sentinel.
func TestReferencePageListsErrors(t *testing.T) {
	t.Parallel()

	raw := mustReadReference(t)
	for _, name := range errorSentinels(t, "errors.go") {
		if !strings.Contains(raw, "`"+name+"`") {
			t.Errorf("sentinel %q missing from reference errors index", name)
		}
	}
}

// mustReadReference reads the battery reference page.
func mustReadReference(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Clean(referenceDoc))
	if err != nil {
		t.Fatalf("read reference page: %v", err)
	}
	return string(raw)
}

// optionKeys walks a struct type collecting dotted JSON keys. Named struct
// fields recurse (redis.prefix); anonymous embeds (shared redisopt-style
// blocks) and "-" tags are skipped.
func optionKeys(t *testing.T, typ reflect.Type, prefix string) []string {
	t.Helper()

	var keys []string
	for i := range typ.NumField() {
		f := typ.Field(i)
		tag, ok := f.Tag.Lookup("json")
		name, _, _ := strings.Cut(tag, ",")
		if !ok || name == "" || name == "-" {
			continue
		}
		if f.Type.Kind() == reflect.Struct {
			keys = append(keys, optionKeys(t, f.Type, prefix+name+".")...)
			continue
		}
		keys = append(keys, prefix+name)
	}
	return keys
}

var errVarPattern = regexp.MustCompile(`(?m)^var (Err[A-Za-z0-9_]+) =`)

// errorSentinels returns the ErrXxx names declared in the given file.
func errorSentinels(t *testing.T, name string) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	matches := errVarPattern.FindAllStringSubmatch(string(raw), -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	if len(names) == 0 {
		t.Fatalf("no ErrXxx sentinels parsed from %s", name)
	}
	return names
}
