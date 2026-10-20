package wire

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  string
		want State
	}{
		{name: "no tag", tag: "", want: StateMissing},
		{name: "other tag only", tag: `zever:"secret"`, want: StateMissing},
		{name: "named", tag: `json:"id"`, want: StateNamed},
		{name: "named with options", tag: `json:"id,omitempty"`, want: StateNamed},
		{name: "empty name", tag: `json:""`, want: StateEmpty},
		{name: "empty name with options", tag: `json:",omitempty"`, want: StateEmpty},
		{name: "dropped", tag: `json:"-"`, want: StateDropped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Classify(tt.tag); got != tt.want {
				t.Fatalf("Classify(%q) = %v, want %v", tt.tag, got, tt.want)
			}
		})
	}
}

func TestWireKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		field  Field
		want   string
		onWire bool
	}{
		{name: "named", field: Field{Name: "ID", Tag: `json:"id"`}, want: "id", onWire: true},
		{name: "named with options", field: Field{Name: "ID", Tag: `json:"id,omitempty"`}, want: "id", onWire: true},
		{name: "empty name falls back to the Go field name", field: Field{Name: "ID", Tag: `json:""`}, want: "ID", onWire: true},
		{name: "missing tag falls back to the Go field name", field: Field{Name: "ID"}, want: "ID", onWire: true},
		{name: "dropped", field: Field{Name: "ID", Tag: `json:"-"`}, want: "-", onWire: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, onWire := tt.field.Key()
			if got != tt.want || onWire != tt.onWire {
				t.Fatalf("WireKey(%+v) = (%q, %v), want (%q, %v)", tt.field, got, onWire, tt.want, tt.onWire)
			}
		})
	}
}

func TestFields(t *testing.T) {
	t.Parallel()

	const source = "package pkg\n\n" +
		"type wire struct {\n" +
		"\tName   string `json:\"name\"`\n" +
		"\tsize   int\n" +
		"\tHidden string `json:\"hidden\"`\n" +
		"\tInner\n" +
		"}\n"

	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, "wire.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}

	fields, ok := Fields(f, "wire")
	if !ok {
		t.Fatal("Fields did not find the wire struct")
	}

	if len(fields) != 2 {
		t.Fatalf("got %d fields, want the 2 exported ones: %+v", len(fields), fields)
	}

	if fields[0].Name != "Name" || fields[0].Tag != `json:"name"` || !fields[0].HasTag {
		t.Errorf("unexpected first field: %+v", fields[0])
	}

	if fields[1].Name != "Hidden" {
		t.Errorf("unexpected second field: %+v", fields[1])
	}

	if _, ok := Fields(f, "missing"); ok {
		t.Error("Fields found a struct that does not exist")
	}
}
