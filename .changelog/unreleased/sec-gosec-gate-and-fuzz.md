## Security Deepening

### GOSEC Gate
- Added curated gosec rule subset to `.golangci.yml` as enforcing gate
- Enabled rules: G101, G102, G106, G107, G108, G201, G202, G203, G303, G305, G307, G401, G403, G501-G505, G601
- Excluded false-positive-heavy rules: G103 (generated protobuf), G104 (unhandled errors), G109-G114 (strconv), G115 (integer overflow), G204 (subprocess), G301/G302/G306 (file perms), G402 (TLS testing), G404 (jitter), G703 (taint)
- Fixed 14 issues: directory permissions (0o750), chmod nolint directives, symlink TOCTOU nolint, unhandled error nolint

### Fuzz Coverage
- **adapters/webhook/queue**: Added `FuzzVerifySignatureHeader`, `FuzzParseSignatureHeader`, `FuzzBuildSignatureHeader` for HMAC-SHA256 envelope verification
- **adapters/storage/local**: Added `FuzzTraversal`, `FuzzPresignKeyValidation`, `FuzzSymlinkContainment` for adversarial path traversal testing
- **adapters/payment/stripe/paddle**: Existing `FuzzWebhookEvent` targets retained
- Registered all new targets in CI `fuzz-smoke` job (20s each)

### Security Bench Matrix
- **adapters/webhook/queue**: `BenchmarkVerifySignatureHeader`, `BenchmarkBuildSignatureHeader` (constant-time HMAC risk signals)
- **adapters/payment/stripe/paddle**: `BenchmarkWebhookEvent` (webhook verification hot path)
- **adapters/auth/jwt**: `BenchmarkVerify`, `BenchmarkVerifyParallel` (token verification)
- **adapters/storage/local**: `BenchmarkHandlerPutTraversal`, `BenchmarkHandlerGetTraversal` (adversarial path handling)
- Delivered `.bench/sec-report.md` with risk lexicon covering 7 Go security classes

### Internal Hardening
- Directory permissions tightened from 0o755 to 0o750 in `tools/gengrammar`, `cmd/zever/skill.go`, `cmd/zever/docs_cmd.go`
- Added explicit `t.Helper()` / `b.Helper()` calls to test helper functions
- Removed unused `#nolint:gosec` directives for excluded rules