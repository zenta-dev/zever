package config

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/shared/codec"
)

type shapeLeaf struct {
	Name  string `json:"name"`
	Token string `json:"tok" zever:"secret"`
}

type shapeOpts struct {
	Ptr       *Secret               `json:"ptr"`
	NilPtr    *Secret               `json:"nil_ptr"`
	Slice     []Secret              `json:"slice"`
	TagSlice  []string              `json:"tag_slice" zever:"secret"`
	Map       map[string]Secret     `json:"map"`
	NilMap    map[string]Secret     `json:"nil_map"`
	TagMap    map[string]string     `json:"tag_map" zever:"secret"`
	Plain     map[string]string     `json:"plain"`
	Nested    shapeLeaf             `json:"nested"`
	NestedPtr *shapeLeaf            `json:"nested_ptr"`
	NilNested *shapeLeaf            `json:"nil_nested"`
	Leaves    []shapeLeaf           `json:"leaves"`
	LeafPtrs  []*shapeLeaf          `json:"leaf_ptrs"`
	LeafMap   map[string]*shapeLeaf `json:"leaf_map"`
	AnyVal    any                   `json:"any_val"`
	AnyStruct any                   `json:"any_struct"`
	AnyNil    any                   `json:"any_nil"`
	AnyMap    map[string]any        `json:"any_map"`
	Anys      []any                 `json:"anys"`
	Direct    Secret                `json:"direct"`
	Empty     Secret                `json:"empty"`
	Keep      string                `json:"keep"`
}

func newShapeOpts() shapeOpts {
	p := Secret("ptr-secret")
	return shapeOpts{
		Ptr:       &p,
		Slice:     []Secret{"s1", "s2"},
		TagSlice:  []string{"t1", "t2"},
		Map:       map[string]Secret{"a": "m1"},
		TagMap:    map[string]string{"a": "tm1"},
		Plain:     map[string]string{"a": "visible"},
		Nested:    shapeLeaf{Name: "n", Token: "nested-secret"},
		NestedPtr: &shapeLeaf{Name: "np", Token: "nestedptr-secret"}, //nolint:gosec // test fixture
		Leaves:    []shapeLeaf{{Name: "l", Token: "leaf-secret"}},
		LeafPtrs:  []*shapeLeaf{{Name: "lp", Token: "leafptr-secret"}, nil},
		LeafMap:   map[string]*shapeLeaf{"k": {Name: "lm", Token: "leafmap-secret"}},
		AnyVal:    Secret("any-secret"),
		AnyStruct: shapeLeaf{Name: "as", Token: "anystruct-secret"}, //nolint:gosec // test fixture
		AnyMap:    map[string]any{"x": Secret("anymap-secret"), "y": "visible"},
		Anys:      []any{Secret("anys-secret"), "visible"},
		Direct:    "direct-secret",
		Keep:      "visible-keep",
	}
}

// secretNeedles are the plaintext values that must never appear in any
// redacted rendering of newShapeOpts.
var secretNeedles = []string{
	"ptr-secret", "s1", "s2", "t1", "t2", "m1", "tm1", "nested-secret",
	"nestedptr-secret", "leaf-secret", "leafptr-secret", "leafmap-secret",
	"any-secret", "anystruct-secret", "anymap-secret", "anys-secret",
	"direct-secret",
}

func assertNoNeedles(t *testing.T, label, rendered string) {
	t.Helper()
	for _, n := range secretNeedles {
		if strings.Contains(rendered, n) {
			t.Errorf("%s leaks %q: %s", label, n, rendered)
		}
	}
}

func TestRedactSecretFields_allShapes(t *testing.T) {
	t.Parallel()

	opts := newShapeOpts()
	m := serviceToMap(opts)
	redactMap(m)
	redactSecretFields(opts, m)

	b, err := codec.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	assertNoNeedles(t, "json", string(b))
	assertNoNeedles(t, "%v", fmt.Sprintf("%v", m))
	assertNoNeedles(t, "%+v", fmt.Sprintf("%+v", m))
	assertNoNeedles(t, "%#v", fmt.Sprintf("%#v", m))

	for _, keep := range []string{"visible-keep", "visible", `"n"`, `"as"`} {
		if !strings.Contains(string(b), strings.Trim(keep, `"`)) {
			t.Errorf("non-secret %s was removed: %s", keep, b)
		}
	}
	if m["empty"] != "" {
		t.Errorf("empty Secret = %v, want empty", m["empty"])
	}
	if m["nil_ptr"] != nil || m["nil_nested"] != nil || m["any_nil"] != nil {
		t.Errorf("nil values must stay nil: %v", m)
	}
	if got, ok := m["tag_slice"].([]any); !ok || len(got) != 2 || got[0] != RedactedValue {
		t.Errorf("tag_slice = %v", got)
	}
}

func TestRedact_typedValuesInMap(t *testing.T) {
	t.Parallel()

	p := Secret("ptr-secret")
	in := map[string]any{
		"direct": Secret("direct-secret"),
		"ptr":    &p,
		"slice":  []Secret{"s1", "s2"},
		"map":    map[string]Secret{"a": "m1"},
		"struct": shapeLeaf{Name: "n", Token: "nested-secret"},
		"structs": []shapeLeaf{
			{Name: "n", Token: "leaf-secret"},
		},
		"nilptr": (*Secret)(nil),
		"empty":  Secret(""),
		"keep":   "visible",
	}
	out := Redact(in)

	b, err := codec.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	assertNoNeedles(t, "Redact", string(b))
	if out["keep"] != "visible" || out["empty"] != "" {
		t.Errorf("unexpected change: %v", out)
	}
	if in["direct"] != Secret("direct-secret") {
		t.Errorf("input mutated: %v", in["direct"])
	}
}

func TestRedactSecretFields_nilAndCycle(t *testing.T) {
	t.Parallel()

	redactSecretFields(nil, map[string]any{"a": "b"})
	redactSecretFields(shapeOpts{}, nil)
	redactSecretFields((*shapeOpts)(nil), map[string]any{"ptr": "x"})
	m := map[string]any{"ptr": "x", "any_val": "y", "leaves": []any{nil}}
	redactSecretFields(shapeOpts{}, m)
	if m["ptr"] != RedactedValue {
		t.Errorf("ptr = %v", m["ptr"])
	}

	type node struct {
		Next *node  `json:"next"`
		Pass Secret `json:"pass"`
	}
	n := &node{Pass: "p1"}
	n.Next = &node{Pass: "p2", Next: n} // cycle; JSON marshal fails -> empty map
	got := serviceToMap(n)
	redactSecretFields(n, got) // must not hang or panic

	acyclic := &node{Pass: "p1", Next: &node{Pass: "p2"}}
	m2 := serviceToMap(acyclic)
	redactSecretFields(acyclic, m2)
	b, _ := codec.Marshal(m2)
	if strings.Contains(string(b), "p1") || strings.Contains(string(b), "p2") {
		t.Errorf("recursive struct leaks: %s", b)
	}

	// Redact on typed cyclic value fails closed rather than hanging.
	out := Redact(map[string]any{"n": n})
	if out["n"] != RedactedValue {
		t.Errorf("cyclic typed value = %v, want redacted", out["n"])
	}
}

func TestRedact_urlishKeysScrubDSNs(t *testing.T) {
	t.Parallel()

	cases := map[string]string{ //nolint:gosec // test fixtures
		"replica_dsn":       "app:pw123@tcp(h:3306)/db",
		"conn_string":       "host=h user=u password=pw123 dbname=d",
		"service_url":       "https://user:pw123@example.com/x?password=pw123",
		"broker_endpoint":   "amqp://guest:pw123@mq:5672/",
		"listen_addr":       "u:pw123@host",
		"mirrors_url":       "",
		"unrelated_comment": "host=h user=u password=visible-by-design",
	}
	for k, v := range cases {
		out := Redact(map[string]any{k: v})
		got, _ := out[k].(string)
		if k == "unrelated_comment" {
			if got != v {
				t.Errorf("%s changed: %q", k, got)
			}
			continue
		}
		if strings.Contains(got, "pw123") {
			t.Errorf("Redact %s = %q leaks", k, got)
		}

		m := map[string]any{k: v}
		redactMap(m)
		if s, _ := m[k].(string); strings.Contains(s, "pw123") {
			t.Errorf("redactMap %s = %q leaks", k, s)
		}
	}

	// String slices under URL-ish keys.
	out := Redact(map[string]any{"mirror_urls": []string{"postgres://u:pw123@h/d", "plain"}})
	if s, ok := out["mirror_urls"].([]string); !ok || strings.Contains(s[0], "pw123") || s[1] != "plain" {
		t.Errorf("slice = %v", s)
	}
	m := map[string]any{"nodes": []any{"redis://:pw123@h:6379"}, "mirror_urls": []string{"postgres://u:pw123@h/d"}}
	redactMap(m)
	if strings.Contains(fmt.Sprint(m), "pw123") {
		t.Errorf("redactMap slices leak: %v", m)
	}
}

func TestRedactedServices_secretShapesViaConfig(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Storage.Options.Secret = "hmac-secret"
	cfg.Auth.Options.JWT.Keys = nil
	svc := cfg.RedactedServices()
	b, err := codec.Marshal(svc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "hmac-secret") {
		t.Errorf("leak: %s", b)
	}
}

// secretGuardAllowlist lists json keys of option fields whose name looks
// secret-bearing but which intentionally lack the secret tag or Secret
// type. Every entry needs a reason; entries marked denylisted must also be
// caught by the key-name denylist.
var secretGuardAllowlist = map[string]struct {
	reason     string
	denylisted bool
}{
	"dsn":                             {"connection string; redacted by key-name denylist and userinfo scrub", true},
	"key_file":                        {"filesystem path, not key material", false},
	"token_file":                      {"filesystem path, not token material", false},
	"tls_key":                         {"filesystem path to a PEM file, not key material", false},
	"key_id":                          {"public key identifier", false},
	"key_ids":                         {"public key identifiers", false},
	"key_length":                      {"numeric parameter", false},
	"allow_dev_key":                   {"boolean flag", false},
	"allow_insecure_token_file_perms": {"boolean flag", false},
	"keys":                            {"element type JWTKey carries a tagged secret field", false},
}

var secretNameRE = regexp.MustCompile(`(?i)password|secret|token|key|dsn`)

func TestOptionsSecretFieldsAreMarked(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob(filepath.Join("..", "core", "*", "options.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no options files found: %v", err)
	}

	used := map[string]bool{}
	fset := token.NewFileSet()
	for _, file := range files {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, fld := range st.Fields.List {
				if fld.Tag == nil {
					continue
				}
				tag := reflect.StructTag(strings.Trim(fld.Tag.Value, "`"))
				key, _, _ := strings.Cut(tag.Get("json"), ",")
				if key == "" || key == "-" || !secretNameRE.MatchString(key) {
					continue
				}
				if tag.Get("zever") == "secret" || typeExprName(fld.Type) == "Secret" {
					continue
				}
				entry, ok := secretGuardAllowlist[key]
				if !ok {
					t.Errorf("%s: field %q looks secret-bearing but has no zever:\"secret\" tag or config.Secret type; tag it or add an allowlist entry with a reason",
						fset.Position(fld.Pos()), key)
					continue
				}
				used[key] = true
				if entry.denylisted && !isSensitiveKey(key) {
					t.Errorf("allowlist %q claims denylist coverage but isSensitiveKey is false", key)
				}
			}
			return true
		})
	}
	for k := range secretGuardAllowlist {
		if !used[k] {
			t.Errorf("stale allowlist entry %q", k)
		}
	}
}

// typeExprName returns the trailing identifier of a type expression,
// looking through pointers, slices, and selectors.
func typeExprName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return typeExprName(x.X)
	case *ast.ArrayType:
		return typeExprName(x.Elt)
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}
