package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func mustBenchServer() *Server { return newServer() }

func mustBenchCallParams(name string, args map[string]any) rpcRequest {
	raw, err := json.Marshal(callParams{Name: name, Arguments: args})
	if err != nil {
		panic(err)
	}
	return rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/call", Params: raw}
}

func stubBenchSchema() *ir.Schema {
	mod := &ir.Module{Name: "blog"}
	ent := &ir.Entity{Name: "Post", Module: mod, Fields: []*ir.Field{
		{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}},
		{Name: "title", Type: ir.FieldType{Scalar: ir.TString}},
	}}
	svc := &ir.Service{Name: "PostService", Module: mod, Operations: []*ir.Operation{
		{Name: "GetPost", Params: []*ir.Param{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}}, Returns: &ir.TypeRef{Entity: ent}},
	}}
	mod.Entities = []*ir.Entity{ent}
	mod.Services = []*ir.Service{svc}
	return &ir.Schema{Modules: []*ir.Module{mod}}
}

func BenchmarkHandlePing(b *testing.B) {
	s := mustBenchServer()
	req := rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "ping"}
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		_ = s.handle(ctx, req)
	}
}

func BenchmarkHandleToolsList(b *testing.B) {
	s := mustBenchServer()
	req := rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/list"}
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		_ = s.handle(ctx, req)
	}
}

func BenchmarkHandleInitialize(b *testing.B) {
	s := mustBenchServer()
	req := rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{"protocolVersion":"2025-06-18"}`)}
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		_ = s.handle(ctx, req)
	}
}

func BenchmarkHandleUnknownMethod(b *testing.B) {
	s := mustBenchServer()
	req := rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "nope"}
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		_ = s.handle(ctx, req)
	}
}

func BenchmarkHandleToolsCallUnknown(b *testing.B) {
	s := mustBenchServer()
	req := mustBenchCallParams("nope", nil)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		_ = s.handle(ctx, req)
	}
}

func BenchmarkHandleToolsCallCompile(b *testing.B) {
	s := mustBenchServer()
	files := testFiles()
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		_ = s.handle(ctx, mustBenchCallParams("zever_compile", map[string]any{"files": files}))
	}
}

func BenchmarkHandleToolsCallSchema(b *testing.B) {
	s := mustBenchServer()
	files := testFiles()
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		_ = s.handle(ctx, mustBenchCallParams("zever_schema", map[string]any{"files": files}))
	}
}

func BenchmarkHandleToolsCallExplain(b *testing.B) {
	s := mustBenchServer()
	files := testFiles()
	ctx := context.Background()
	// Resolve module name once outside loop.
	sumResp := s.handle(ctx, mustBenchCallParams("zever_schema", map[string]any{"files": files}))
	res, _ := sumResp.Result.(callToolResult)
	var sum schemaSummary
	_ = json.Unmarshal([]byte(res.Content[0].Text), &sum)
	path := sum.Modules[0].Name + ".Post"
	b.ReportAllocs()
	for b.Loop() {
		_ = s.handle(ctx, mustBenchCallParams("zever_explain", map[string]any{"files": files, "path": path}))
	}
}

func BenchmarkToolList(b *testing.B) {
	s := mustBenchServer()
	b.ReportAllocs()
	for b.Loop() {
		_ = s.toolList()
	}
}

func BenchmarkErrorResponseEncode(b *testing.B) {
	id := json.RawMessage("1")
	b.ReportAllocs()
	for b.Loop() {
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(errorResponse(id, codeMethodNotFound, "zever-mcp: method not found: x"))
	}
}

func BenchmarkDecodeArgs(b *testing.B) {
	args := map[string]any{"dir": "", "files": testFiles(), "path": "blog.Post"}
	b.ReportAllocs()
	for b.Loop() {
		var out explainInput
		_ = decodeArgs(args, &out)
	}
}

func BenchmarkSummarize(b *testing.B) {
	schema := stubBenchSchema()
	b.ReportAllocs()
	for b.Loop() {
		_ = summarize(schema)
	}
}

func BenchmarkSummarizeMarshal(b *testing.B) {
	schema := stubBenchSchema()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = json.Marshal(summarize(schema))
	}
}

func BenchmarkExplainEntity(b *testing.B) {
	schema := stubBenchSchema()
	ctx := b.Context()
	_ = ctx
	b.ReportAllocs()
	for b.Loop() {
		_, _ = explain(schema, "blog.Post")
	}
}

func BenchmarkExplainOperation(b *testing.B) {
	schema := stubBenchSchema()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = explain(schema, "blog.PostService.GetPost")
	}
}

func BenchmarkScalarName(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = scalarName(ir.TString)
	}
}

func BenchmarkFieldTypeString(b *testing.B) {
	ft := ir.FieldType{Scalar: ir.TEnum, EnumName: "Role"}
	b.ReportAllocs()
	for b.Loop() {
		_ = fieldTypeString(ft)
	}
}

func BenchmarkDescribeEntity(b *testing.B) {
	schema := stubBenchSchema()
	ent := schema.Modules[0].Entities[0]
	b.ReportAllocs()
	for b.Loop() {
		_ = describeEntity(ent)
	}
}

func BenchmarkFormatDiags(b *testing.B) {
	s := mustBenchServer()
	ctx := context.Background()
	resp := s.handle(ctx, mustBenchCallParams("zever_compile", map[string]any{"files": map[string]string{"b/b.zen": "entity {"}}))
	_ = resp
	b.ReportAllocs()
	for b.Loop() {
		_ = formatDiags(nil)
	}
}
