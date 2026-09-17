# internal/dsl

Compiler frontend for the zever schema language: source text → resolved
schema (`*ir.Schema`) plus formatting, compatibility checking, and the
codegen extension point. Compiler-internal: runtime packages must not
import it (mirrors the `internal/` rule of the original design).

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
| `backend` | `Backend` interface only; backends land in later phases. |
| `gengrammar` | Renders `editors/nvim` and `editors/vscode` grammar files from `token.Keywords` / `resolver.ScalarTypeNames`; run via `make generate` (`tools/gengrammar`). |

Entry points: `parser.New(file, src).ParseFile()` →
`resolver.Resolve(files)` → `*ir.Schema`; or `compile.Compile(files,
backends...)` for the full path with `compile/testdata/app.zen` as the
canonical fixture (plus `internal/dsl/testdata/` fuzz seeds).

## Notes

- Fuzz tests (`lexer/parser/resolver`) run seed corpora as unit tests.
- `format.OffsetOf` retains one provably unreachable guard (kept verbatim).
- Parser `parseArg`/`parseFieldDecl` hold three provably unreachable
  branches (documented in `parser_gap_test.go`); package measures 99.7%,
  everything reachable covered.
