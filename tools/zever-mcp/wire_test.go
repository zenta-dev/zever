package main

import (
	"testing"

	"github.com/zenta-dev/zever/shared/codec"
)

// These tests pin the JSON key names of the zever-mcp tool payloads
// (moduleSummary and the resource bundle). A stripped or renamed tag
// changes the wire shape MCP clients parse while still compiling.

func TestModuleSummaryWireKeys(t *testing.T) {
	t.Parallel()

	m := moduleSummary{
		Name:     "billing",
		Version:  "v0.8.0",
		Entities: []string{"Order", "Invoice"},
	}
	out, err := codec.Marshal(m)
	if err != nil {
		t.Fatalf("marshal moduleSummary: %v", err)
	}

	// name/version/entities present; messages/enums/services omitted (omitempty).
	want := `{"name":"billing","version":"v0.8.0","entities":["Order","Invoice"]}`
	if string(out) != want {
		t.Errorf("marshal moduleSummary:\n got %s\nwant %s", out, want)
	}
}

func TestResourceBundleWireKeys(t *testing.T) {
	t.Parallel()

	// The resource bundle emitted for zever_resources pins its two keys.
	bundle := struct {
		Dir   string            `json:"dir"`
		Files map[string]string `json:"files"`
	}{Dir: "schema", Files: map[string]string{"app.zen": "contents"}}

	out, err := codec.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal resource bundle: %v", err)
	}

	want := `{"dir":"schema","files":{"app.zen":"contents"}}`
	if string(out) != want {
		t.Errorf("marshal resource bundle:\n got %s\nwant %s", out, want)
	}
}
