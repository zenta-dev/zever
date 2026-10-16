# WS8 documentation system: IA pilot (pr-ws8)

Status: proposal + 3-page pilot implemented in this PR. Follow-ups do the mass moves.

## Goal

Organize the docs site into four quadrants (see `contribute/writing-docs`,
which already defines these page types):

| Quadrant | Question | Lives in | Contains |
|---|---|---|---|
| Tutorial | Teach me | `tutorials/`, `start/` | Linear, runnable, no side trips |
| How-to guide | Help me do X | `guides/` | Task-first, assumes the basics |
| Reference | Tell me the exact fact | `reference/` | Neutral, complete, scannable |
| Explanation | Help me understand | `concepts/` | Why, no step lists |

## Current state (this PR)

- `tutorials/` (overview + 3 app builds) and `start/` (installation,
  quickstart, project layout, versioning) already read as tutorial/onboarding.
- `guides/` (http, data, background, security, operate, integrations,
  extend) already reads as how-to.
- `concepts/` already reads as explanation.
- `reference/` reads as reference, except it hosted one how-to
  (`agent-skill`: "Install and use"), moved to `guides/extend/` in this PR.
- The dead `digging-deeper/` section had no sidebar entry and two orphan
  pages added after the last restructure; both move to `guides/` in this PR.
- `docs/ia-map.json` (legacy URL map) gains the two `digging-deeper`
  strays; `docs/astro.config.mjs` gains exact redirects for all three moves.

## Pilot moves (implemented)

| # | Old URL | New URL | Rationale |
|---|---|---|---|
| 1 | `digging-deeper/cross-service-tracing` | `guides/operate/cross-service-tracing` | Operate runbook content; pairs with the observability guide. No sidebar entry before; now under Operate. |
| 2 | `digging-deeper/outbox-ops` | `guides/background/outbox-ops` | Companion how-to to the outbox guide (metrics, alerts, DLQ runbook). Now under Background work. |
| 3 | `reference/agent-skill` | `guides/extend/agent-skill` | "Install and use" is help-me-do-X, not look-it-up. Now under Extend. |

URL stability: every old URL keeps working through an exact redirect in
`docs/astro.config.mjs` (plus the pre-existing generic
`/digging-deeper/` catch-all). Inbound relative links updated in the same PR:
`guides/http/middleware`, `guides/operate/deployment`,
`guides/operate/observability`, `guides/background/outbox`,
`reference/cli`, `tutorials/build-an-agent-app`, `reference/index`,
landing `Hero`/`AgentsSection`/`landing.ts`.

## Full map (proposed, NOT implemented)

Quadrant homes for everything else stay as-is: no further moves are needed
for the section skeleton. The remaining WS8 work is per-battery reference
coverage, largest-first risk order:

1. `auth`, `db`/`queue`/`cache` (security- and data-loss-relevant go first).
2. The remaining 30 services, following the generator pattern below.
3. Errors indexes for the same batteries (one table per page, drift-tested).

Deferred (needs maintainer call, not in pilot):

- `guides/extend/customizing-and-leaving`: hybrid (ownership model +
  reference table + tasks). Proposal: split into concept
  (`concepts/ownership`), reference table (fold into config/matrix), and
  keep the task halves as guides. Do not move whole.
- `reference/agent-skill` footer grouping: the landing footer still lists
  it under Reference with the corrected path; regroup if marketing wants.
- `guides/operate/testing-your-app`: patterns catalog, correctly in
  Operate; its Contributor-rules tail could move to `contribute/`.

## Generator pattern (how to add battery N)

1. Copy `docs/src/content/docs/reference/flag.mdx` to
   `docs/src/content/docs/reference/<battery>.mdx`. Keep the five sections
   (Adapters, Configuration, Options, Errors, Related). Every config
   key, flag, adapter, and package named must exist in code; verify by
   grep before committing. No `go` code fences (the doc-snippets guard
   resolves every zever import/symbol); yaml and sh fences are unchecked.
2. Copy `core/flag/reference_test.go` to `core/<battery>/reference_test.go`:
   swap the package name, the `referenceDoc` path, the `Adapter` list, and
   the service key. The option-key walker recurses named blocks and skips
   `-`/anonymous embeds; the sentinel parser reads `errors.go` for
   `^var ErrXxx`. Session shows the escape hatch for adapters registered
   outside core (`sessionDBAdapter` with its source comment).
3. Wire sidebar (`docs/astro.config.mjs` Reference items, after Config)
   and a card in `docs/src/content/docs/reference/index.mdx`.
4. Run: drift tests (`go test ./core/<battery>/ -run TestReferencePage`),
   `gofmt -l`, `tools/check-doc-snippets.sh`.

## Verification

- `go test ./core/flag/ ./core/ratelimit/ ./core/session/ -run TestReferencePage -count=1`: green.
- `tools/check-doc-snippets.sh`: green (new pages carry no `go` fences).
- `npm run build` in `docs/`: TODO(verify) — not run here (offline npm).
