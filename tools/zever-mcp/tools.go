package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zenta-dev/zever/dsl/backend/openapi"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// compileInput is the shared schema source accepted by every tool. Exactly one
// of Dir or Files is required.
type compileInput struct {
	Dir   string            `json:"dir"`
	Files map[string]string `json:"files"`
}

// decodeArgs re-encodes a tool argument map into a typed struct.
func decodeArgs(args map[string]any, out any) error {
	raw, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// compileSchema resolves the schema source and compiles it with the OpenAPI
// backend. A non-nil result is returned even when diagnostics carry errors, so
// callers can format them.
func compileSchema(ctx context.Context, in compileInput) (*compile.Result, diag.List, error) {
	files := in.Files
	if len(files) == 0 {
		if in.Dir == "" {
			return nil, nil, errors.New("either dir or files is required")
		}

		read, err := readZenFiles(in.Dir)
		if err != nil {
			return nil, nil, err
		}
		files = read
	}

	res, diags := compile.CompileContext(ctx, files, openapi.New())
	return res, diags, nil
}

// readZenFiles reads every .zen file directly under dir.
func readZenFiles(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zen") {
			continue
		}

		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		files[e.Name()] = string(b)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .zen files in %s", dir)
	}

	return files, nil
}

// formatDiags renders diagnostics as file:line:col: message lines.
func formatDiags(diags diag.List) string {
	var b strings.Builder
	for _, d := range diags.Sorted() {
		fmt.Fprintf(&b, "%s:%d:%d: %s\n", d.Pos.File, d.Pos.Line, d.Pos.Col, d.Msg)
	}
	return b.String()
}

// objectSchema builds a JSON Schema object from a property map.
func objectSchema(props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props}
}

// compileTool compiles .zen sources and returns the merged OpenAPI document.
func compileTool() toolDef {
	return toolDef{
		Name:        "zever_compile",
		Description: "Compile .zen schema sources and return the merged OpenAPI 3.0.3 document as JSON.",
		InputSchema: objectSchema(map[string]any{
			"dir": map[string]any{
				"type":        "string",
				"description": "Directory containing .zen files.",
			},
			"files": map[string]any{
				"type":                 "object",
				"description":          "Inline schema files keyed by filename.",
				"additionalProperties": map[string]any{"type": "string"},
			},
		}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			var in compileInput
			if err := decodeArgs(args, &in); err != nil {
				return "", err
			}

			res, diags, err := compileSchema(ctx, in)
			if err != nil {
				return "", err
			}
			if diags.HasErrors() {
				return "", errors.New(formatDiags(diags))
			}

			out := res.Outputs["openapi"]["openapi.json"]
			if len(out) == 0 {
				return "", errors.New("openapi backend produced no merged document")
			}

			return string(out), nil
		},
	}
}

// moduleSummary is a compact description of one schema module.
type moduleSummary struct {
	Name     string           `json:"name"`
	Version  string           `json:"version,omitempty"`
	Entities []string         `json:"entities,omitempty"`
	Messages []string         `json:"messages,omitempty"`
	Enums    []string         `json:"enums,omitempty"`
	Services []serviceSummary `json:"services,omitempty"`
}

// serviceSummary is a compact description of one service.
type serviceSummary struct {
	Name       string   `json:"name"`
	Operations []string `json:"operations,omitempty"`
}

// schemaSummary is the resolved-schema overview returned by zever_schema.
type schemaSummary struct {
	Modules []moduleSummary `json:"modules"`
}

// summarize builds a schemaSummary from a resolved schema.
func summarize(s *ir.Schema) schemaSummary {
	var out schemaSummary

	for _, m := range s.Modules {
		ms := moduleSummary{Name: m.Name, Version: m.Version}
		for _, e := range m.Entities {
			ms.Entities = append(ms.Entities, e.Name)
		}
		for _, msg := range m.Messages {
			ms.Messages = append(ms.Messages, msg.Name)
		}
		for _, en := range m.Enums {
			ms.Enums = append(ms.Enums, en.Name)
		}
		for _, svc := range m.Services {
			ss := serviceSummary{Name: svc.Name}
			for _, op := range svc.Operations {
				ss.Operations = append(ss.Operations, op.Name)
			}
			ms.Services = append(ms.Services, ss)
		}
		out.Modules = append(out.Modules, ms)
	}

	return out
}

// schemaTool resolves .zen sources and returns a JSON schema overview.
func schemaTool() toolDef {
	return toolDef{
		Name:        "zever_schema",
		Description: "Resolve .zen schema sources and return a JSON overview of modules, entities, services, messages and enums.",
		InputSchema: objectSchema(map[string]any{
			"dir": map[string]any{
				"type":        "string",
				"description": "Directory containing .zen files.",
			},
			"files": map[string]any{
				"type":                 "object",
				"description":          "Inline schema files keyed by filename.",
				"additionalProperties": map[string]any{"type": "string"},
			},
		}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			var in compileInput
			if err := decodeArgs(args, &in); err != nil {
				return "", err
			}

			res, diags, err := compileSchema(ctx, in)
			if err != nil {
				return "", err
			}
			if diags.HasErrors() {
				return "", errors.New(formatDiags(diags))
			}

			b, err := json.MarshalIndent(summarize(res.Schema), "", "  ")
			if err != nil {
				return "", err
			}

			return string(b), nil
		},
	}
}

// explainInput names a schema element by dotted path.
type explainInput struct {
	Dir   string            `json:"dir"`
	Files map[string]string `json:"files"`
	Path  string            `json:"path"`
}

// explainTool describes a module, entity, message, service or operation.
func explainTool() toolDef {
	return toolDef{
		Name:        "zever_explain",
		Description: "Explain a schema element by dotted path: Module, Module.Entity, Module.Service or Module.Service.Operation.",
		InputSchema: objectSchema(map[string]any{
			"dir": map[string]any{
				"type":        "string",
				"description": "Directory containing .zen files.",
			},
			"files": map[string]any{
				"type":                 "object",
				"description":          "Inline schema files keyed by filename.",
				"additionalProperties": map[string]any{"type": "string"},
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Dotted path to explain, e.g. blog.Post or blog.PostService.GetPost.",
			},
		}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			var in explainInput
			if err := decodeArgs(args, &in); err != nil {
				return "", err
			}
			if in.Path == "" {
				return "", errors.New("path is required")
			}

			res, diags, err := compileSchema(ctx, compileInput{Dir: in.Dir, Files: in.Files})
			if err != nil {
				return "", err
			}
			if diags.HasErrors() {
				return "", errors.New(formatDiags(diags))
			}

			return explain(res.Schema, in.Path)
		},
	}
}

// explain renders a dotted path against a resolved schema.
func explain(s *ir.Schema, path string) (string, error) {
	parts := strings.Split(path, ".")

	mod := findModule(s, parts[0])
	if mod == nil {
		return "", fmt.Errorf("module %q not found", parts[0])
	}

	switch len(parts) {
	case 1:
		return fmt.Sprintf("module %s: %d entities, %d services, %d messages, %d enums",
			mod.Name, len(mod.Entities), len(mod.Services), len(mod.Messages), len(mod.Enums)), nil
	case 2:
		if e := findEntity(mod, parts[1]); e != nil {
			return describeEntity(e), nil
		}
		if svc := findService(mod, parts[1]); svc != nil {
			return describeService(svc), nil
		}
		if msg := findMessage(mod, parts[1]); msg != nil {
			return describeMessage(msg), nil
		}
		return "", fmt.Errorf("%q not found in module %q", parts[1], mod.Name)
	case 3:
		svc := findService(mod, parts[1])
		if svc == nil {
			return "", fmt.Errorf("service %q not found in module %q", parts[1], mod.Name)
		}
		op := findOperation(svc, parts[2])
		if op == nil {
			return "", fmt.Errorf("operation %q not found in service %q", parts[2], svc.Name)
		}
		return describeOperation(op), nil
	default:
		return "", fmt.Errorf("path %q is too deep", path)
	}
}

func findModule(s *ir.Schema, name string) *ir.Module {
	for _, m := range s.Modules {
		if m.Name == name {
			return m
		}
	}
	return nil
}

func findEntity(m *ir.Module, name string) *ir.Entity {
	for _, e := range m.Entities {
		if e.Name == name {
			return e
		}
	}
	return nil
}

func findMessage(m *ir.Module, name string) *ir.Message {
	for _, msg := range m.Messages {
		if msg.Name == name {
			return msg
		}
	}
	return nil
}

func findService(m *ir.Module, name string) *ir.Service {
	for _, svc := range m.Services {
		if svc.Name == name {
			return svc
		}
	}
	return nil
}

func findOperation(svc *ir.Service, name string) *ir.Operation {
	for _, op := range svc.Operations {
		if op.Name == name {
			return op
		}
	}
	return nil
}

func describeEntity(e *ir.Entity) string {
	var b strings.Builder
	fmt.Fprintf(&b, "entity %s (%d fields):\n", e.Name, len(e.Fields))
	for _, f := range e.Fields {
		fmt.Fprintf(&b, "- %s: %s\n", f.Name, fieldTypeString(f.Type))
	}
	return b.String()
}

func describeMessage(m *ir.Message) string {
	var b strings.Builder
	fmt.Fprintf(&b, "message %s (%d fields):\n", m.Name, len(m.Fields))
	for _, f := range m.Fields {
		fmt.Fprintf(&b, "- %s: %s\n", f.Name, fieldTypeString(f.Type))
	}
	return b.String()
}

func describeService(svc *ir.Service) string {
	var b strings.Builder
	fmt.Fprintf(&b, "service %s (%d operations):\n", svc.Name, len(svc.Operations))
	for _, op := range svc.Operations {
		fmt.Fprintf(&b, "- %s\n", describeOperation(op))
	}
	return b.String()
}

func describeOperation(op *ir.Operation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s(", op.Name)
	for i, p := range op.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s: %s", p.Name, fieldTypeString(p.Type))
	}
	b.WriteString(")")
	if op.Returns != nil {
		fmt.Fprintf(&b, " -> %s", op.Returns.Name())
	}
	return b.String()
}

func fieldTypeString(ft ir.FieldType) string {
	if ft.Scalar == ir.TEnum {
		if ft.EnumName != "" {
			return "enum " + ft.EnumName
		}
		return "enum(" + strings.Join(ft.EnumValues, ",") + ")"
	}
	return scalarName(ft.Scalar)
}

func scalarName(s ir.ScalarType) string {
	switch s {
	case ir.TUUID:
		return "uuid"
	case ir.TString:
		return "string"
	case ir.TInt32:
		return "int32"
	case ir.TInt64:
		return "int64"
	case ir.TFloat32:
		return "float32"
	case ir.TFloat64:
		return "float64"
	case ir.TBool:
		return "bool"
	case ir.TTimestamp:
		return "timestamp"
	case ir.TDate:
		return "date"
	case ir.TBytes:
		return "bytes"
	case ir.TJSON:
		return "json"
	case ir.TEnum:
		return "enum"
	default:
		return "unknown"
	}
}
