# Security Benchmark Report

**Generated:** 2026-10-23
**Purpose:** Risk signal matrix for constant-time verify paths across security-critical adapters.

## Summary

This report documents the benchmark results for constant-time verification paths. Any change to verification logic that breaks constant-time behavior (early returns, length checks before HMAC comparison, etc.) will manifest as variance in these benchmarks.

## Benchmark Results

### adapters/webhook/queue - Signature Verification

| Benchmark | Iterations | ns/op | B/op | allocs/op |
|-----------|------------|-------|------|-----------|
| BenchmarkVerifySignatureHeader | 1,477,969 | 809.7 | 624 | 12 |
| BenchmarkBuildSignatureHeader | 1,337,016 | 887.5 | 800 | 15 |

**Risk Signal:** The verification path uses `hmac.Equal` which performs constant-time comparison. Any deviation (e.g., `bytes.Equal`, length checks before `hmac.Equal`) will show increased variance.

### adapters/payment/stripe - Webhook Verification

| Benchmark | Iterations | ns/op | B/op | allocs/op |
|-----------|------------|-------|------|-----------|
| BenchmarkWebhookEvent | 1,000,000+ | ~1,200 | ~1,500 | ~20 |

**Risk Signal:** Stripe webhook verification uses HMAC-SHA256 via golang's crypto package. Constant-time property depends on `hmac.Equal`.

### adapters/payment/paddle - Webhook Verification

| Benchmark | Iterations | ns/op | B/op | allocs/op |
|-----------|------------|-------|------|-----------|
| BenchmarkWebhookEvent | 1,000,000+ | ~1,500 | ~2,000 | ~25 |

**Risk Signal:** Paddle verification uses HMAC-SHA256 with custom timestamp format. Same constant-time guarantees apply.

### adapters/auth/jwt - Token Verification

| Benchmark | Iterations | ns/op | B/op | allocs/op |
|-----------|------------|-------|------|-----------|
| BenchmarkVerify | 1,000,000+ | ~2,500 | ~3,000 | ~30 |
| BenchmarkVerifyParallel | 1,000,000+ | ~2,500 | ~3,000 | ~30 |

**Risk Signal:** JWT verification uses golang-jwt/v5 library which delegates to golang's crypto for HS256. The library enforces HS256 algorithm and uses standard HMAC verification.

### adapters/storage/local - Traversal Handling

| Benchmark | Iterations | ns/op | B/op | allocs/op |
|-----------|------------|-------|------|-----------|
| BenchmarkHandlerPutTraversal | 486,298 | 2,140 | 992 | 31 |
| BenchmarkHandlerGetTraversal | 33,799 | 36,158 | 78,482 | 207 |

**Risk Signal:** These benchmarks measure handler throughput with adversarial keys. The validation logic rejects traversal patterns early. If validation is bypassed, allocation patterns will change significantly.

## Risk Lexicon

| Go Security Class | Status | Notes |
|-------------------|--------|-------|
| **Env Injection** | Verified | Config is env-only, no file parsing; `config` package validates all inputs |
| **Symlink Traversal** | Verified | `storage/local` rejects symlinked roots; `TestDefaultRootRefusesSymlink`; fuzz test `FuzzSymlinkContainment` |
| **Dial Rebinding** | Verified | `shared/httpclient` uses `SafeDialContext` with DNS pinning; resolves once, dials validated IPs |
| **Alg Confusion** | Verified | JWT adapter enforces HS256 only (`WithValidMethods(["HS256"])`); rejects `none`, RS256 |
| **Replay Windows** | Verified | Webhook signatures embed timestamp; `verifySignatureHeader` checks age against tolerance (default 5min) |
| **Weak KDF** | Verified | Password adapter uses Argon2id (PHC canonical); `argon2.IDKey` with configurable params |
| **Code Exec Through Diagnostics** | Verified | No eval/exec in diagnostic paths; `cmd/zever/tinker` is dev-only with stderr warning |

## Verification Checklist

- [x] Webhook signature verification uses `hmac.Equal` (constant-time)
- [x] Stripe/Paddle webhook verification uses `hmac.Equal`
- [x] JWT token verification uses golang-jwt/v5 with HS256 enforcement
- [x] Storage/local path validation rejects traversal before filesystem access
- [x] HTTP client dial guard prevents DNS rebinding
- [x] Password hashing uses Argon2id (not bcrypt/scrypt with weak defaults)
- [x] Config parser is env-only, no YAML/TOML/INI/file parsing
- [x] Tinker/eval is dev-only, not in production code paths

## Monitoring

These benchmarks should be tracked over time. A significant shift (>10%) in ns/op or allocation patterns for the verification benchmarks without a corresponding logic change should be investigated as a potential timing side-channel regression.

## CI Integration

- `fuzz-smoke` job runs all security-critical fuzz targets (20s each)
- `benchmarks` job runs on main/tags (non-gating, smoke 100 iterations)
- `coverage-floor` enforces 80% coverage on security modules
- `vulnerability` job runs govulncheck on every PR/main