# dsl

Compiler frontend for the zever schema language: source text → resolved
schema (`*ir.Schema`) plus formatting, compatibility checking, and the
codegen extension point. Compiler-internal: runtime packages must not
import it (layering rule for the top-level `dsl` module).

## Pipeline

```
source text → lexer → parser → ast → resolver → ir.Schema
                                    ↘ compile → backend outputs
```

| Package | Role |
|---|---|
| `diag` | Positions, diagnostics, multi-error lists. Foundation, zero deps. |
| `token` | Token vocabulary + keywords. |
| `lexer` | Bytes → tokens. Never panics; comment side-channel; fuzz-tested. |
| `ast` | Pure-data syntax tree, positions on every node. |
| `parser` | Best-effort parse with per-decl recovery + depth guard. |
| `naming` | Case converters for backends. |
| `resolver` | Multi-pass `[]*ast.File` → `*ir.Schema`, keep-going diagnostics. |
| `ir` | Resolved schema + gRPC-canonical `ErrorCode` table. |
| `format` | Token-gap formatter; refuses lex-dirty input; idempotent. |
| `breaking` | Old-vs-new schema compatibility (`Change` taxonomy). |
| `compile` | Filename-sorted parse → resolve → `backend.Backend.Generate`. |
| `backend` | `Backend`/`ContextBackend` interfaces plus the shipped backends: `atlas` (DDL/schema), `gogen` (Go server code), `mcp` (MCP server manifest), `openapi`, `proto` (protobuf messages/services), `protogogen` (generated `.pb.go`/gRPC stubs: drives `protoc-gen-go` as a library and `go tool protoc-gen-go-grpc` as a subprocess — no system protoc), `zenorm` (ORM modules). |
| `gengrammar` | Renders `editors/nvim` and `editors/vscode` grammar files from `token.Keywords` / `resolver.ScalarTypeNames`; run via `make generate` (`tools/gengrammar`). |

Entry points: `parser.New(file, src).ParseFile()` →
`resolver.Resolve(files)` → `*ir.Schema`; or `compile.Compile(files,
backends...)` for the full path with `compile/testdata/app.zen` as the
canonical fixture (plus `dsl/testdata/` fuzz seeds).

## Notes

- Fuzz tests (`lexer/parser/resolver`) run seed corpora as unit tests.
- `format.OffsetOf` retains one provably unreachable guard (kept verbatim).
- Parser `parseArg`/`parseFieldDecl` hold three provably unreachable
  branches (documented in `parser_gap_test.go`); package measures 99.7%,
  everything reachable covered.
