# Track D - Type Safety + DX - Todo

## 1. Type-Safety Inventory: Fix //nolint:forcetypeassert sites

### shared/lrucache (15 sites)
- [ ] lrucache.go - 7 sites
- [ ] ttl_ext.go - 8 sites
- **Approach**: Strengthen nolint comments (container invariant - package controls all mutations)

### orm/option.go (9 sites)
- [ ] convertScan function - 9 forced type assertions
- **Approach**: Restructure to use type-safe conversion or add comma-ok with error

### orm/inchunks.go (2 sites)
- [ ] line 153 - target.Value.([]any)
- [ ] line 192 - best.Value.([]any)
- **Approach**: Add comma-ok assertions with errors (should never fail due to logic invariants)

### adapters/crypto/kms/kms.go (1 site)
- [ ] line 291 - d.signPriv.Public().(ed25519.PublicKey)
- **Approach**: This is a stdlib guarantee - strengthen nolint comment

### adapters/crypto/local/local.go (1 site)
- [ ] line 148 - l.signPriv.Public().(ed25519.PublicKey)
- **Approach**: This is a stdlib guarantee - strengthen nolint comment

## 2. Add Missing Compile-Time Interface Satisfaction Asserts

### Missing in adapters (from grep check):
- [ ] ai/anthropic - missing `var _ ai.AI = (*adapter)(nil)`
- [ ] ai/ollama - missing
- [ ] ai/openai - missing
- [ ] analytics/log - missing
- [ ] analytics/posthog - missing
- [ ] billing/paddle - missing
- [ ] crypto/kms - missing
- [ ] crypto/local - missing
- [ ] db/sqlite - missing
- [ ] document/local - missing
- [ ] document/remote - missing
- [ ] geo/google - missing
- [ ] geo/osm - missing
- [ ] geo/static - missing
- [ ] lock/memory - missing
- [ ] lock/redis - missing
- [ ] log/noop - missing
- [ ] log/pretty - missing
- [ ] log/slog - missing
- [ ] log/zerolog - missing
- [ ] media/ffmpeg - missing
- [ ] media/local - missing
- [ ] media/s3 - missing
- [ ] outbox/cdc - missing
- [ ] outbox/db - missing
- [ ] password/argon2 - missing
- [ ] payment/paddle - missing
- [ ] payment/stripe - missing
- [ ] payment/stub - missing
- [ ] permission/casbin - missing
- [ ] queue/memory - missing
- [ ] queue/redis - missing
- [ ] resilience/inproc - missing
- [ ] resilience/redis - missing
- [ ] router/fiber - missing
- [ ] router/stdhttp - missing
- [ ] scheduler/embedded - missing
- [ ] search/meilisearch - missing
- [ ] secrets/env - missing
- [ ] secrets/vault - missing
- [ ] session/cookie - missing
- [ ] storage/local - missing
- [ ] storage/r2 - missing
- [ ] storage/s3 - missing
- [ ] tenant/header - missing
- [ ] tenant/single - missing
- [ ] vectorstore/qdrant - missing
- [ ] webhook/http - missing
- [ ] webhook/queue - missing
- [ ] workflow/memory - missing

## 3. DX Hardening Sweep

### Error Suggestion Paths Consistency
- [ ] Verify every battery's ParseAdapter error includes did-you-mean where config path does
- [ ] Check that all ParseAdapter functions use the same pattern

### Doctor/CLI Hint Texts
- [ ] Verify doctorFixHint covers all batteries correctly
- [ ] Check for any missing or incorrect hint texts

### docs/bench/README.md
- [ ] Create documentation explaining benchmark story

## 4. Report Table
- [ ] Document before/after type-safety posture
- [ ] Count of nolint sites fixed
- [ ] Count of added asserts
- [ ] Linter additions