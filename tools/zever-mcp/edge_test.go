package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func mustServer(t *testing.T) *Server {
	t.Helper()
	return newServer()
}

func TestEdge_invalidJSONRPCVersion(t *testing.T) {
	t.Parallel()

	s := mustServer(t)
	for _, v := range []string{"", "1.0", "2.1"} {
		resp := s.handle(t.Context(), rpcRequest{JSONRPC: v, ID: json.RawMessage("1"), Method: "ping"})
		if resp.Error == nil || resp.Error.Code != codeInvalidRequest {
			t.Errorf("version %q: error = %+v, want invalid request", v, resp.Error)
		}
		if !strings.Contains(resp.Error.Message, "zever-mcp:") {
			t.Errorf("version %q: message %q missing prefix", v, resp.Error.Message)
		}
	}
}

func TestEdge_ping(t *testing.T) {
	t.Parallel()

	s := mustServer(t)
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("7"), Method: "ping"})
	if resp.Error != nil {
		t.Fatalf("ping error = %+v", resp.Error)
	}
	m, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("ping result type = %T", resp.Result)
	}
	if len(m) != 0 {
		t.Errorf("ping result = %v, want empty", m)
	}
}

func TestEdge_initializeFallbacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params string
		want   string
	}{
		{name: "empty params", params: "", want: defaultProtocolVersion},
		{name: "empty object", params: `{}`, want: defaultProtocolVersion},
		{name: "unknown version", params: `{"protocolVersion":"1999-01-01"}`, want: defaultProtocolVersion},
		{name: "supported old", params: `{"protocolVersion":"2024-11-05"}`, want: "2024-11-05"},
		{name: "supported new", params: `{"protocolVersion":"2025-06-18"}`, want: "2025-06-18"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := mustServer(t)
			resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(tc.params)})
			if resp.Error != nil {
				t.Fatalf("error = %+v", resp.Error)
			}
			m, ok := resp.Result.(map[string]any)
			if !ok {
				t.Fatalf("result type = %T", resp.Result)
			}
			if m["protocolVersion"] != tc.want {
				t.Errorf("protocolVersion = %v, want %s", m["protocolVersion"], tc.want)
			}
		})
	}
}

func TestEdge_initializeInvalidParams(t *testing.T) {
	t.Parallel()

	s := mustServer(t)
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{"protocolVersion":`)})
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("error = %+v, want invalid params", resp.Error)
	}
}

func TestEdge_toolsCallMalformedParams(t *testing.T) {
	t.Parallel()

	s := mustServer(t)
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/call", Params: json.RawMessage(`{oops`)})
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("error = %+v, want invalid params", resp.Error)
	}
}

func TestEdge_toolsCallEmptyName(t *testing.T) {
	t.Parallel()

	s := mustServer(t)
	raw, err := json.Marshal(callParams{Name: "", Arguments: nil})
	if err != nil {
		t.Fatal(err)
	}
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/call", Params: raw})
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("error = %+v, want invalid params for empty tool", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "unknown tool") {
		t.Errorf("message = %q, want unknown tool", resp.Error.Message)
	}
}

func TestEdge_handlerErrorMapsToIsError(t *testing.T) {
	t.Parallel()

	s := &Server{tools: map[string]toolDef{}}
	s.register(toolDef{
		Name:        "boom",
		Description: "d",
		InputSchema: objectSchema(nil),
		Handler: func(_ context.Context, _ map[string]any) (string, error) {
			return "", errors.New("boom: kaput")
		},
	})
	raw, err := json.Marshal(callParams{Name: "boom"})
	if err != nil {
		t.Fatal(err)
	}
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/call", Params: raw})
	if resp.Error != nil {
		t.Fatalf("handler error must be tool error, not rpc error: %+v", resp.Error)
	}
	res := toolResult(t, resp)
	if !res.IsError {
		t.Fatal("want IsError")
	}
	if !strings.Contains(res.Content[0].Text, "kaput") {
		t.Errorf("text = %q", res.Content[0].Text)
	}
}

func TestEdge_isNotification(t *testing.T) {
	t.Parallel()

	if (rpcRequest{ID: nil}).isNotification() != true {
		t.Error("nil ID should be notification")
	}
	if (rpcRequest{ID: json.RawMessage("")}).isNotification() != true {
		t.Error("empty ID should be notification")
	}
	if (rpcRequest{ID: json.RawMessage("null")}).isNotification() {
		t.Error("null ID must not be notification (has id member)")
	}
	if (rpcRequest{ID: json.RawMessage("1")}).isNotification() {
		t.Error("numeric ID must not be notification")
	}
}

func TestEdge_errorResponseShape(t *testing.T) {
	t.Parallel()

	resp := errorResponse(json.RawMessage("3"), codeMethodNotFound, "zever-mcp: x")
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q", resp.JSONRPC)
	}
	if string(resp.ID) != "3" {
		t.Errorf("id = %s", resp.ID)
	}
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("error = %+v", resp.Error)
	}
}

func TestEdge_runParseError(t *testing.T) {
	t.Parallel()

	var out, errb bytes.Buffer
	code := run(t.Context(), strings.NewReader("{oops\n"), &out, &errb)
	if code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	var resp rpcResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != codeParseError {
		t.Fatalf("error = %+v, want parse error", resp.Error)
	}
	if !strings.Contains(errb.String(), "zever-mcp:") {
		t.Errorf("stderr = %q, want prefix", errb.String())
	}
}

func TestEdge_runEmptyInputEOF(t *testing.T) {
	t.Parallel()

	var out, errb bytes.Buffer
	if code := run(t.Context(), strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("run = %d, want 0", code)
	}
	if out.Len() != 0 {
		t.Errorf("out = %q, want empty", out.String())
	}
}

type stubFailWriter struct{ err error }

func (w stubFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestEdge_runEncodeFailure(t *testing.T) {
	t.Parallel()

	in := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n"
	var errb bytes.Buffer
	code := run(t.Context(), strings.NewReader(in), stubFailWriter{err: errors.New("nope")}, &errb)
	if code != 1 {
		t.Fatalf("run = %d, want 1 on encode failure", code)
	}
	if !strings.Contains(errb.String(), "zever-mcp: encode:") {
		t.Errorf("stderr = %q, want encode prefix", errb.String())
	}
}

func TestEdge_decodeArgsFailure(t *testing.T) {
	t.Parallel()

	var out compileInput
	if err := decodeArgs(map[string]any{"dir": make(chan int)}, &out); err == nil {
		t.Fatal("expected marshal error for chan value")
	}
}

func TestEdge_decodeArgsRoundTrip(t *testing.T) {
	t.Parallel()

	var out explainInput
	err := decodeArgs(map[string]any{"path": "a.B", "files": map[string]string{"a/a.zen": "x"}}, &out)
	if err != nil {
		t.Fatalf("decodeArgs: %v", err)
	}
	if out.Path != "a.B" {
		t.Errorf("path = %q", out.Path)
	}
}

func TestEdge_compileSchemaNoSource(t *testing.T) {
	t.Parallel()

	_, _, err := compileSchema(t.Context(), compileInput{})
	if err == nil || !strings.Contains(err.Error(), "either dir or files") {
		t.Fatalf("err = %v, want dir-or-files", err)
	}
}

func TestEdge_compileSchemaFilesWinOverDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.zen"), []byte("entity {{"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, diags, err := compileSchema(t.Context(), compileInput{Dir: dir, Files: testFiles()})
	if err != nil {
		t.Fatalf("compileSchema: %v", err)
	}
	if diags.HasErrors() || res == nil {
		t.Fatal("files must take precedence over dir")
	}
}

func TestEdge_compileSchemaMissingDir(t *testing.T) {
	t.Parallel()

	_, _, err := compileSchema(t.Context(), compileInput{Dir: filepath.Join(t.TempDir(), "nope")})
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestEdge_readZenFiles(t *testing.T) {
	t.Parallel()

	t.Run("missing dir", func(t *testing.T) {
		t.Parallel()
		if _, err := readZenFiles(filepath.Join(t.TempDir(), "nope")); err == nil {
			t.Error("want error")
		}
	})

	t.Run("no zen files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := readZenFiles(dir)
		if err == nil || !strings.Contains(err.Error(), "no .zen files") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("skips subdir and non-zen", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sub", "x.zen"), []byte(testSchema), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ok.zen"), []byte(testSchema), 0o600); err != nil {
			t.Fatal(err)
		}
		files, err := readZenFiles(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 || files["ok.zen"] == "" {
			t.Fatalf("files = %v, want only ok.zen", files)
		}
	})

	t.Run("oversize rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		big := bytes.Repeat([]byte("x"), maxSchemaFileBytes+1)
		if err := os.WriteFile(filepath.Join(dir, "big.zen"), big, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := readZenFiles(dir)
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Errorf("err = %v, want exceeds", err)
		}
	})
}

func TestEdge_formatDiagsEmpty(t *testing.T) {
	t.Parallel()

	if got := formatDiags(nil); got != "" {
		t.Errorf("formatDiags(nil) = %q, want empty", got)
	}
}

func TestEdge_objectSchema(t *testing.T) {
	t.Parallel()

	m := objectSchema(nil)
	if m["type"] != "object" {
		t.Errorf("type = %v", m["type"])
	}
	m2 := objectSchema(map[string]any{"a": "b"})
	props, ok := m2["properties"].(map[string]any)
	if !ok || props["a"] != "b" {
		t.Errorf("props = %v", m2["properties"])
	}
}

func mustSchema(t *testing.T) *ir.Schema {
	t.Helper()
	s := mustServer(t)
	resp := callToolReq(t, s, "zever_schema", map[string]any{"files": testFiles()})
	var sum schemaSummary
	if err := json.Unmarshal([]byte(resultText(t, resp)), &sum); err != nil {
		t.Fatalf("summary: %v", err)
	}
	// Recompile to get resolved IR directly.
	res, diags, err := compileSchema(t.Context(), compileInput{Files: testFiles()})
	if err != nil {
		t.Fatalf("compileSchema: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("diags: %v", formatDiags(diags))
	}
	_ = sum
	return res.Schema
}

func TestEdge_explainBranches(t *testing.T) {
	t.Parallel()

	schema := mustSchema(t)
	mod := schema.Modules[0].Name

	tests := []struct {
		name      string
		path      string
		wantSub   string
		wantError string
	}{
		{name: "module", path: mod, wantSub: "module "},
		{name: "entity", path: mod + ".Post", wantSub: "entity Post"},
		{name: "service", path: mod + ".PostService", wantSub: "service PostService"},
		{name: "operation", path: mod + ".PostService.GetPost", wantSub: "GetPost("},
		{name: "unknown module", path: "nope", wantError: "not found"},
		{name: "unknown member", path: mod + ".Nope", wantError: "not found"},
		{name: "unknown service for op", path: mod + ".Nope.Op", wantError: "not found"},
		{name: "too deep", path: mod + ".PostService.GetPost.Extra", wantError: "too deep"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := explain(schema, tc.path)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("err = %v, want %q", err, tc.wantError)
				}
				if !strings.Contains(err.Error(), "zever-mcp:") {
					t.Errorf("err = %v missing prefix", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("explain: %v", err)
			}
			if !strings.Contains(got, tc.wantSub) {
				t.Errorf("got = %q, want %q", got, tc.wantSub)
			}
		})
	}
}

func TestEdge_explainUnknownOperation(t *testing.T) {
	t.Parallel()

	schema := mustSchema(t)
	mod := schema.Modules[0].Name
	_, err := explain(schema, mod+".PostService.Nope")
	if err == nil || !strings.Contains(err.Error(), "operation") {
		t.Fatalf("err = %v, want operation not found", err)
	}
}

func TestEdge_explainToolRequiresPath(t *testing.T) {
	t.Parallel()

	s := mustServer(t)
	resp := callToolReq(t, s, "zever_explain", map[string]any{"files": testFiles()})
	res := toolResult(t, resp)
	if !res.IsError {
		t.Fatal("want isError when path missing")
	}
	if !strings.Contains(res.Content[0].Text, "path is required") {
		t.Errorf("text = %q", res.Content[0].Text)
	}
}

func TestEdge_explainToolBadSchema(t *testing.T) {
	t.Parallel()

	s := mustServer(t)
	resp := callToolReq(t, s, "zever_explain", map[string]any{
		"files": map[string]string{"b/b.zen": "entity {"},
		"path":  "b.Thing",
	})
	if res := toolResult(t, resp); !res.IsError {
		t.Fatal("want isError for bad schema")
	}
}

func TestEdge_summarizeEmpty(t *testing.T) {
	t.Parallel()

	sum := summarize(&ir.Schema{})
	if len(sum.Modules) != 0 {
		t.Errorf("modules = %d, want 0", len(sum.Modules))
	}
	b, err := json.Marshal(sum)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "modules") {
		t.Errorf("json = %s", b)
	}
}

func TestEdge_findNil(t *testing.T) {
	t.Parallel()

	schema := mustSchema(t)
	mod := schema.Modules[0]
	if findModule(schema, "nope") != nil {
		t.Error("findModule want nil")
	}
	if findEntity(mod, "nope") != nil {
		t.Error("findEntity want nil")
	}
	if findMessage(mod, "nope") != nil {
		t.Error("findMessage want nil")
	}
	if findService(mod, "nope") != nil {
		t.Error("findService want nil")
	}
	svc := findService(mod, "PostService")
	if svc == nil {
		t.Fatal("PostService missing")
	}
	if findOperation(svc, "nope") != nil {
		t.Error("findOperation want nil")
	}
	if findOperation(svc, "GetPost") == nil {
		t.Error("GetPost missing")
	}
}

func TestEdge_scalarNameAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   ir.ScalarType
		want string
	}{
		{ir.TUUID, "uuid"}, {ir.TString, "string"}, {ir.TInt32, "int32"},
		{ir.TInt64, "int64"}, {ir.TFloat32, "float32"}, {ir.TFloat64, "float64"},
		{ir.TBool, "bool"}, {ir.TTimestamp, "timestamp"}, {ir.TDate, "date"},
		{ir.TBytes, "bytes"}, {ir.TJSON, "json"}, {ir.TEnum, "enum"},
		{ir.ScalarType(999), "unknown"},
	}
	for _, tc := range tests {
		if got := scalarName(tc.in); got != tc.want {
			t.Errorf("scalarName(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEdge_fieldTypeString(t *testing.T) {
	t.Parallel()

	if got := fieldTypeString(ir.FieldType{Scalar: ir.TString}); got != "string" {
		t.Errorf("got %q", got)
	}
	if got := fieldTypeString(ir.FieldType{Scalar: ir.TEnum, EnumName: "Role"}); got != "enum Role" {
		t.Errorf("got %q", got)
	}
	if got := fieldTypeString(ir.FieldType{Scalar: ir.TEnum, EnumValues: []string{"a", "b"}}); !strings.Contains(got, "a") {
		t.Errorf("got %q", got)
	}
	if got := fieldTypeString(ir.FieldType{Scalar: ir.TEnum}); got != "enum()" {
		t.Errorf("got %q, want enum()", got)
	}
}

func TestEdge_describeHelpers(t *testing.T) {
	t.Parallel()

	schema := mustSchema(t)
	mod := schema.Modules[0]
	ent := findEntity(mod, "Post")
	if ent == nil {
		t.Fatal("Post missing")
	}
	if got := describeEntity(ent); !strings.Contains(got, "entity Post") || !strings.Contains(got, "title") {
		t.Errorf("describeEntity = %q", got)
	}
	svc := findService(mod, "PostService")
	if got := describeService(svc); !strings.Contains(got, "service PostService") || !strings.Contains(got, "GetPost") {
		t.Errorf("describeService = %q", got)
	}
	op := findOperation(svc, "GetPost")
	if got := describeOperation(op); !strings.Contains(got, "GetPost(") {
		t.Errorf("describeOperation = %q", got)
	}
	// Operation without returns.
	if got := describeOperation(&ir.Operation{Name: "Ping"}); got != "Ping()" {
		t.Errorf("got %q", got)
	}
	// Message describe.
	msg := &ir.Message{Name: "M", Fields: []*ir.Field{{Name: "f", Type: ir.FieldType{Scalar: ir.TBool}}}}
	if got := describeMessage(msg); !strings.Contains(got, "message M") {
		t.Errorf("describeMessage = %q", got)
	}
}

func TestEdge_toolListOrder(t *testing.T) {
	t.Parallel()

	s := mustServer(t)
	got := s.toolList()
	want := []string{"zever_compile", "zever_schema", "zever_explain", "zever_doctor", "zever_generate"}
	if len(got) != len(want) {
		t.Fatalf("tools = %d", len(got))
	}
	for i, w := range want {
		if got[i]["name"] != w {
			t.Errorf("tools[%d] = %v, want %s", i, got[i]["name"], w)
		}
	}
}

func TestEdge_registerPreservesOrder(t *testing.T) {
	t.Parallel()

	s := &Server{tools: map[string]toolDef{}}
	s.register(toolDef{Name: "b"})
	s.register(toolDef{Name: "a"})
	if len(s.order) != 2 || s.order[0] != "b" || s.order[1] != "a" {
		t.Errorf("order = %v", s.order)
	}
}

func TestEdge_runSkipsNotificationOnly(t *testing.T) {
	t.Parallel()

	in := "{\"jsonrpc\":\"2.0\",\"method\":\"note\"}\n"
	var out, errb bytes.Buffer
	if code := run(t.Context(), strings.NewReader(in), &out, &errb); code != 0 {
		t.Fatalf("run = %d", code)
	}
	if out.Len() != 0 {
		t.Errorf("out = %q, want none", out.String())
	}
}

func TestEdge_runUnknownMethodResponds(t *testing.T) {
	t.Parallel()

	in := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"nope\"}\n"
	var out, errb bytes.Buffer
	if code := run(t.Context(), strings.NewReader(in), &out, &errb); code != 0 {
		t.Fatalf("run = %d", code)
	}
	var resp rpcResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("error = %+v", resp.Error)
	}
}

func TestEdge_schemaToolBadSource(t *testing.T) {
	t.Parallel()

	s := mustServer(t)
	resp := callToolReq(t, s, "zever_schema", map[string]any{})
	if res := toolResult(t, resp); !res.IsError {
		t.Fatal("want isError")
	}
}

func TestEdge_compileToolNoOutput(t *testing.T) {
	t.Parallel()

	// Empty-but-valid compile input path: files key present but empty map
	// falls back to dir check and errors.
	s := mustServer(t)
	resp := callToolReq(t, s, "zever_compile", map[string]any{"files": map[string]string{}})
	if res := toolResult(t, resp); !res.IsError {
		t.Fatal("want isError")
	}
}

var _ = io.EOF
