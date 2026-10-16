# Threat model: router

Audited against `core/router/` (`router.go`, `options.go`), `core/middleware`
(`SecurityHeaders`, CORS, `ClientIPKey`, `Recover`), and the generated server
(`cmd/zever/generate_server.go`) at WS6 time.

## Assets

- Every request/response crossing the process edge: credentials, cookies,
  tokens, PII. The listener socket itself (plaintext vs TLS).

## Trust boundaries

- The network is untrusted. `X-Forwarded-For` and other proxy headers are
  attacker-controlled unless the direct peer is a trusted proxy.
- Generated-server defaults aim safe, but the operator owns the edge
  (TLS termination, proxy topology).

## What zever guarantees

- TLS options are presence-paired at validation: `tls_cert` requires
  `tls_key` and vice versa; `tls_client_ca` (mTLS) requires both. The
  generated server builds one shared `tls.Config` with a TLS 1.2 floor and
  serves HTTP and gRPC from it; unpaired material fails fast with a fix hint.
- Serving plaintext on a non-loopback address logs one warning per listener;
  plaintext loopback stays quiet as the local-dev default.
- `middleware.SecurityHeaders` (wired innermost in the generated server) sets
  `X-Content-Type-Options: nosniff`, a locked-down `Permissions-Policy`, and
  `Referrer-Policy` by default; `X-Frame-Options: SAMEORIGIN` by default with
  opt-in CSP; HSTS only with a positive max age, and only when TLS is
  configured, so plaintext dev servers never emit HSTS.
- `ClientIPKey` reads every `X-Forwarded-For` line and keys on the rightmost
  untrusted entry; spoofed headers from untrusted peers are ignored, and
  IPv4-mapped IPv6 trusted ranges are normalized so they match either form.
- gRPC reflection is off by default; it wires only with
  `GRPC_REFLECTION=true` or a dev `ZEVER_ENV` (explicit `false` always wins).
- `ValidMethod` constrains registered methods to known HTTP verbs.

## What the caller must do

- Terminate TLS at or before the app in production; treat the plaintext
  warning as an incident, not noise. Configure `trusted_proxies` so rate
  limiting and IP keys see the real client; never trust `X-Forwarded-For`
  from the open internet.
- Set a CSP appropriate to the app (framing, scripts); the default ships
  none because only the app knows its resource policy.
- Keep reflection off in production. Validate route patterns (unsafe HTTP
  paths are rejected by ongoing hardening; treat route tables as reviewable
  security surface).

## Explicit non-guarantees

- The router does not authenticate, authorize, or rate-limit by itself:
  `auth`/`authz`/ratelimit middleware are separate batteries the app must wire.
- `SecurityHeaders` is a baseline, not a policy: it cannot know the app's
  framing, script, or HSTS-preload needs.
