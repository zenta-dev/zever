package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/shared/codec"
)

func TestCheckToolOK(t *testing.T) {
	t.Parallel()

	res := callToolReq(t, newServer(), "zever_check", map[string]any{"files": testFiles()})

	text := resultText(t, res)
	if toolResult(t, res).IsError || !strings.HasPrefix(text, "ok: 1 module(s)") {
		t.Fatalf("check = %q (isError=%v), want ok summary", text, toolResult(t, res).IsError)
	}
}

func TestCheckToolReportsDiagnostics(t *testing.T) {
	t.Parallel()

	files := map[string]string{"bad.zen": "entity Broken {\n  x: nonexistent_type\n}\n"}

	res := callToolReq(t, newServer(), "zever_check", map[string]any{"files": files})
	if !toolResult(t, res).IsError {
		t.Fatalf("expected isError for an invalid schema, got %q", resultText(t, res))
	}

	if text := resultText(t, res); !strings.Contains(text, "bad.zen:2:") || !strings.Contains(text, "nonexistent_type") {
		t.Fatalf("diagnostics lack position or type: %q", text)
	}
}

func TestCheckToolNeedsSource(t *testing.T) {
	t.Parallel()

	if !toolResult(t, callToolReq(t, newServer(), "zever_check", map[string]any{})).IsError {
		t.Fatal("expected isError when neither dir nor files is given")
	}
}

func TestRoutesTool(t *testing.T) {
	t.Parallel()

	res := callToolReq(t, newServer(), "zever_routes", map[string]any{"files": testFiles()})

	var got routesResult
	if err := codec.Unmarshal([]byte(resultText(t, res)), &got); err != nil {
		t.Fatalf("routes output is not JSON: %v\n%s", err, resultText(t, res))
	}

	if got.Count != 1 || len(got.Routes) != 1 {
		t.Fatalf("routes = %+v, want exactly one", got)
	}

	r := got.Routes[0]
	if r.Method != http.MethodGet || r.Path != "/v1/posts/{id}" || !strings.HasSuffix(r.Target, ".PostService.GetPost") {
		t.Fatalf("route = %+v", r)
	}
}

func TestRoutesToolFromDirAndEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.zen"), []byte("entity Thing {\n  id: uuid @primary\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := callToolReq(t, newServer(), "zever_routes", map[string]any{"dir": dir})

	var got routesResult
	if err := codec.Unmarshal([]byte(resultText(t, res)), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}

	if got.Count != 0 || got.Routes == nil {
		t.Fatalf("routes = %+v, want empty non-nil list", got)
	}
}

func TestRoutesToolInvalidSchema(t *testing.T) {
	t.Parallel()

	res := callToolReq(t, newServer(), "zever_routes", map[string]any{"files": map[string]string{"bad.zen": "entity {"}})
	if !toolResult(t, res).IsError {
		t.Fatal("expected isError for an invalid schema")
	}
}
