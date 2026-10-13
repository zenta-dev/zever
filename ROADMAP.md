# Roadmap to 1.0

Pre-1.0 focus: correctness and security hardening first, DX and API
discipline alongside; 1.0 ships only when the criteria below hold.

## 1.0 criteria checklist

All three must hold (see [STABILITY.md](STABILITY.md#10-promotion-criteria)
for the binding definitions):

- [ ] **6 months without a core break:** no breaking change to any Core
  package in the 6 months before the 1.0 tag, measured from `CHANGELOG.md`
  release-note entries. (Not started: core breaks still permitted with
  release notes.)
- [ ] **100% core kits:** every Core battery with an adapter registry ships
  a conformance kit and every shipped adapter runs it green in CI.
  (In progress: 40 kit packages ship, 33 with in-kit suites; `outbox` and
  `resilience` kits run outside the conformance gate — see
  [STABILITY.md](STABILITY.md#current-standing-honest).)
- [ ] **External production user:** at least one application outside this
  repo runs a tagged release in production. (None known.)

## After 1.0

Core graduates to the 1.0 compatibility promise (SemVer: breaking changes
only in majors, deprecation window per [STABILITY.md](STABILITY.md)).
Extended packages keep their own pace; adapters come and go behind stable
facades.
