package config

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestSchemaMatchesConfig guards against drift between the Go option structs
// reached from Config and the hand-authored config/schema/zever.schema.json.
// It reflects over every Service[T] field of Config (skipping Plugins, which
// is a map), flattens each Options struct to dotted leaf paths, and asserts
// every path is present in the schema. It also asserts the schema's service
// names equal knownServiceNames().
func TestSchemaMatchesConfig(t *testing.T) {
	t.Parallel()

	goLeaves := goOptionLeaves(t, reflect.TypeOf(Config{}))
	schemaLeaves, schemaServices := schemaOptionLeaves(t)

	// Presence is matched case-insensitively: retry.Policy and permission.Rule
	// carry no struct tags, so encoding/json matches their exported Go field
	// names case-insensitively (retry keys are declared lowercase, permission
	// keys capitalized). Snake_case still differs from a Go field name, so real
	// drift such as base_delay vs basedelay is still caught.
	schemaSet := make(map[string]struct{}, len(schemaLeaves))
	for _, p := range schemaLeaves {
		schemaSet[strings.ToLower(p)] = struct{}{}
	}

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "every config option path is in the schema",
			run: func(t *testing.T) {
				t.Helper()

				var missing []string
				for _, p := range goLeaves {
					if _, ok := schemaSet[strings.ToLower(p)]; !ok {
						missing = append(missing, p)
					}
				}
				if len(missing) > 0 {
					sort.Strings(missing)
					t.Errorf("schema missing %d config option path(s):\n%s", len(missing), strings.Join(missing, "\n"))
				}
			},
		},
		{
			name: "schema service names match knownServiceNames",
			run: func(t *testing.T) {
				t.Helper()

				want := knownServiceNames()
				sort.Strings(want)
				got := append([]string(nil), schemaServices...)
				sort.Strings(got)
				if !slices.Equal(got, want) {
					t.Errorf("schema service names:\n got: %v\nwant: %v", got, want)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

// TestSchemaCoversAdapterBatteries asserts every adapters/<battery>/*/register.go
// directory has a matching service property in the schema, so a new battery
// cannot ship without a schema entry.
func TestSchemaCoversAdapterBatteries(t *testing.T) {
	t.Parallel()

	_, schemaServices := schemaOptionLeaves(t)
	serviceSet := make(map[string]struct{}, len(schemaServices))
	for _, s := range schemaServices {
		serviceSet[s] = struct{}{}
	}

	root := filepath.Join("..", "adapters")
	matches, err := filepath.Glob(filepath.Join(root, "*", "*", "register.go"))
	if err != nil {
		t.Fatalf("glob adapter register.go: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no adapter register.go found under %s (run from the config module)", root)
	}

	for _, m := range matches {
		rel, err := filepath.Rel(root, m)
		if err != nil {
			t.Fatalf("relative path for %s: %v", m, err)
		}
		battery := strings.Split(rel, string(filepath.Separator))[0]
		if _, ok := serviceSet[battery]; !ok {
			t.Errorf("adapter battery %q has no service property in the schema", battery)
		}
	}
}

// jsonName returns the primary json key of a struct field, "" when the field
// has no json tag, and "-" when it is explicitly omitted.
func jsonName(f reflect.StructField) string {
	tag, ok := f.Tag.Lookup("json")
	if !ok || tag == "" {
		return ""
	}
	return strings.Split(tag, ",")[0]
}

// goOptionLeaves flattens every Service[T].Options reachable from cfgType to
// dotted leaf paths such as "crypto.options.key_id".
func goOptionLeaves(t *testing.T, cfgType reflect.Type) []string {
	t.Helper()

	var leaves []string
	for i := 0; i < cfgType.NumField(); i++ {
		field := cfgType.Field(i)
		if field.Name == "Plugins" {
			continue
		}
		name := jsonName(field)
		if name == "" || name == "-" {
			t.Fatalf("config field %s has no usable json tag", field.Name)
		}
		opts, ok := field.Type.FieldByName("Options")
		if !ok {
			t.Fatalf("config service %s has no Options field", name)
		}
		walkOptionType(name+".options", opts.Type, &leaves)
	}
	sort.Strings(leaves)
	return leaves
}

// walkOptionType appends the decodable JSON leaf paths of struct type typ under
// prefix. Embedded structs are flattened with no path segment; fields tagged
// json:"-" are skipped; untagged fields use the lowercased Go field name.
func walkOptionType(prefix string, typ reflect.Type, out *[]string) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		if jsonName(field) == "-" {
			continue
		}

		ft := field.Type
		if field.Anonymous && jsonName(field) == "" {
			base := ft
			if base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			if base.Kind() == reflect.Struct {
				walkOptionType(prefix, base, out)
				continue
			}
		}

		name := jsonName(field)
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		walkOptionField(prefix+"."+name, ft, out)
	}
}

// walkOptionField records path as a leaf, or recurses when the field is a plain
// struct. Custom json.Marshaler types are leaves (their wire shape differs from
// their Go fields, e.g. storage.PolicyConfig). Interface/func fields are not
// file-decodable and produce no path.
func walkOptionField(path string, typ reflect.Type, out *[]string) {
	if implementsJSONMarshaler(typ) {
		*out = append(*out, path)
		return
	}

	base := typ
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}

	kind := base.Kind()
	if kind == reflect.Struct {
		walkOptionType(path, base, out)
		return
	}
	if kind == reflect.Interface || kind == reflect.Func || kind == reflect.Chan || kind == reflect.UnsafePointer {
		// Not decodable from a config file; no schema key.
		return
	}
	*out = append(*out, path)
}

var jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

func implementsJSONMarshaler(typ reflect.Type) bool {
	return typ.Implements(jsonMarshalerType) || reflect.PointerTo(typ).Implements(jsonMarshalerType)
}

// schemaOptionLeaves decodes the embedded SchemaJSON and returns the flattened
// dotted leaf paths under every service's options plus the service names.
func schemaOptionLeaves(t *testing.T) ([]string, []string) {
	t.Helper()

	var root map[string]any
	dec := json.NewDecoder(bytes.NewReader(SchemaJSON))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		t.Fatalf("decode SchemaJSON: %v", err)
	}

	defs, ok := root["$defs"].(map[string]any)
	if !ok {
		t.Fatal("SchemaJSON has no $defs object")
	}
	props, ok := root["properties"].(map[string]any)
	if !ok {
		t.Fatal("SchemaJSON has no properties object")
	}

	services := make([]string, 0, len(props))
	var leaves []string
	for name, raw := range props {
		node, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("schema service %s is not an object", name)
		}
		svcProps, ok := node["properties"].(map[string]any)
		if !ok {
			t.Fatalf("schema service %s has no properties object", name)
		}
		opts, ok := svcProps["options"].(map[string]any)
		if !ok {
			t.Fatalf("schema service %s has no options object", name)
		}
		services = append(services, name)
		flattenSchemaOptions(defs, name+".options", opts, &leaves)
	}
	sort.Strings(services)
	sort.Strings(leaves)
	return leaves, services
}

// flattenSchemaOptions resolves $refs recursively and appends a leaf path for
// every node without named properties (scalars, arrays, and free-form objects).
func flattenSchemaOptions(defs map[string]any, prefix string, node map[string]any, out *[]string) {
	if ref, ok := node["$ref"].(string); ok {
		if resolved := resolveSchemaRef(defs, ref); resolved != nil {
			flattenSchemaOptions(defs, prefix, resolved, out)
			return
		}
		*out = append(*out, prefix)
		return
	}

	sub, ok := node["properties"].(map[string]any)
	if !ok || len(sub) == 0 {
		*out = append(*out, prefix)
		return
	}
	for key, child := range sub {
		childNode, ok := child.(map[string]any)
		if !ok {
			continue
		}
		flattenSchemaOptions(defs, prefix+"."+key, childNode, out)
	}
}

func resolveSchemaRef(defs map[string]any, ref string) map[string]any {
	const marker = "#/$defs/"
	if !strings.HasPrefix(ref, marker) {
		return nil
	}
	resolved, ok := defs[strings.TrimPrefix(ref, marker)].(map[string]any)
	if !ok {
		return nil
	}
	return resolved
}
