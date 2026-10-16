# Mutation-testing pilot: dsl/parser and orm/render

Date: 2026-10-16. Scope: WS7 testing-and-quality pilot.
Method: manual (documented) mutant set — no mutation tool was installable
offline (`go-mutesting`/`gremlins` need network for toolchain deps), so each
mutant below was applied by hand to the worktree, the owning package suite
was run with `go test -count=1`, and the worktree was reverted with
`git checkout -- <file>` before the next mutant. Baseline for both packages
was green before the experiment.

Status key: KILLED = suite failed on the mutant (good); SURVIVED = suite
stayed green (gap — worth a new test).

## dsl/parser mutants

| ID | Location | Mutation | Status |
|----|----------|----------|--------|
| P1 | `parser.go` `isSagaDot` | `dot == pos` → `dot != pos` (retract wrong lex diags) | KILLED — saga tests fail |
| P2 | `parser_attribute.go` `parseValue` | depth guard `>` → `>=` (off-by-one) | SURVIVED — only a depth-5000 test exists, blind to the boundary |
| P3 | `parser_attribute.go` `parseIntLitToken` | `sign * n` → `n` (drop negative sign) | KILLED — negative-int test fails |
| P4 | `parser.go` `syncTopLevel` | remove `if depthBraces > 0` guard (decrement below zero) | KILLED — recovery tests fail |
| P5 | `parser.go` `docCommentFor` | attach line `pos.Line-1` → `pos.Line` | KILLED — doc-comment tests fail |
| P6 | `parser_attribute.go` `parseAttribute` | remove empty-parens fast path (always `parseArgList`) | SURVIVED — no test parses `@name()` with empty `()` |

## orm/render mutants

| ID | Location | Mutation | Status |
|----|----------|----------|--------|
| R1 | `render.go` `validateSelectModifiers` | drop `DistinctOn` from the DISTINCT+lock check | KILLED — `TestSelectModsDistinctOnWithLockErrors` fails |
| R2 | `render.go` `validateTablesample` | range `> 100` → `>= 100` (reject 100) | SURVIVED — tests use 10/5.5 and reject -1/101 only |
| R3 | `render.go` `lockClause` | `SKIP LOCKED` wins over `NOWAIT` when both set | SURVIVED — both-flags case never rendered |
| R4 | `render.go` `tablesampleClause` | drop `strings.ToUpper` normalization | SURVIVED — tests only use uppercase methods |
| R5 | `render.go` `validateTablesample` | range `\|\|` → `&&` (check never fires) | KILLED — percentage-range test fails |

## New tests killing the top survivors (5)

1. `TestParseValueDepthBoundary` (`dsl/parser/parser_mutation_test.go`) kills P2:
   199-deep nesting parses clean, 200-deep records "max nesting depth
   exceeded" (calibrated: depth accounting is nestings+1, guard is 200).
2. `TestParseAttributeEmptyParens` (same file) kills P6: `@a()` parses with
   zero args and no diagnostics.
3. `TestSelectModsTablesampleHundredValid` (`orm/render/tablesample_mutation_test.go`)
   kills R2: `Arg: 0` and `Arg: 100` render (`TABLESAMPLE SYSTEM (100)`).
4. `TestSelectModsLockNoWaitWinsOverSkipLocked` (same file) kills R3: both
   flags render `FOR UPDATE NOWAIT`.
5. `TestSelectModsTablesampleLowercaseNormalized` (same file) kills R4:
   `Method: "bernoulli"` renders `TABLESAMPLE BERNOULLI (5.5)`.

## Worth killing later (not in this pilot)

- `argCounter.next` start value (`$0` first placeholder) — covered today by
  the placeholder-numbering test, but only through `SelectMods`; the
  projected/CTE paths share the counter and deserve a direct pin.
- `syncBlock` `&&` → `||` (stop recovery too early) — killed today, but only
  indirectly via multi-decl recovery tests; a single-member-block case would
  pin it directly.
- `parseDurationLit` returning `nil` on invalid duration instead of a
  `Valid:false` node — no mutant was tried here; needs a probe.

## Tooling note

Re-running this pilot with `gremlins` (or `go-mutesting`) once network is
available would widen the mutant set beyond hand-picked boundaries; the table
above is the seed corpus for that run. Gremlins' default operators cover
exactly the classes that survived here (condition-boundary, arithmetic).
