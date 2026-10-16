# WS6 Security — PR tracking

- Branch: `agent/ws6-security` (worktree
  `/home/vee/Zenta/zever/.worktrees/agent-ws6-security`, based on origin/main).
- PR: https://github.com/zenta-dev/zever/pull/639
  (`docs(security): core threat models and gosec audit`, base `main`).
- Label: `needs-human-security-review` (created; "Blocked on human security
  review; do not merge").
- Scope: docs-only + CHANGELOG. Files: 13× `docs/security/threat-model-*.md`,
  `docs/adrs/2026-10-16-asymmetric-jwt.md` (proposal), `docs/plans/`
  scorecard + cosign drafts (not wired), `.github/SECURITY.md` additions,
  `CHANGELOG.md` Unreleased Security entry.
- Overlap check: name-only diffs of #617 (release/installer), #618/#619
  (session), #620 (sslmode), #622 (recover redact), #608 (storage), #610
  (oidc) reviewed — no duplication; open proposals cited as unmerged in the
  threat models.
- Audit: every `nolint:gosec` in scope recorded in PR body; one fix-later
  (postgres pool `int32` clamp); code untouched per workstream rule.
- Verify: `gofmt -l` clean; `docs.yml` only builds `docs/**` (Astro), so no
  docs-build impact; links checked by inspection.
- Status: NOT merge-ready without human review (standing requirement for
  this workstream).
