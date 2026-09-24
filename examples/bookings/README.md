# Bookings example

Short-term space bookings sample built on the zever framework.
This PR covers schema + CLI-generated output only. No business logic,
no handlers yet.

## Layout

- `schema/bookings.zen` — entities (User, Space, Booking, Review),
  BookingService RPCs, jobs (SendConfirmation, SendReminder) + Reminder schedule.
- `generated/` — CLI output (`--backend=zenorm,proto,protogogen,gogen,openapi,atlas`).
  Never hand-edit; regenerate with `zever compile`.
- `internal/api/testdata/schema.sql` — DDL via `zever db migrate --dry-run --adapter=sqlite`.
- `zever.yaml` — explicit zero-infra service adapters for later PRs.
- `data/` — local sqlite path (git-kept empty).

## Runbook outline

1. Migrate: `zever db migrate --adapter=sqlite --dsn=data/bookings.db schema/bookings.zen`
2. Seed: coming in a later PR.
3. Serve: coming in a later PR (`zever serve --config zever.yaml`).
4. Worker: coming in a later PR (queue `default`/`low` consumers for
   SendConfirmation/SendReminder + embedded scheduler for Reminder cron).

## Doctor note

`zever doctor --config zever.yaml` reports FAILs that are expected with
zero-infra defaults, not framework bugs (do not fix framework code):

- ai, auth, geo, i18n: same 4 FAILs as bare defaults (missing api key,
  jwt secret, cities.json, embed FS).
- db: `data/bookings.db` resolves relative to `examples/bookings/`; run doctor
  from that directory or after migrating.
- webhook (queue variant): needs its queue reference, wired in a later PR.

## Worker, seed, webhooks (this PR)

Run from `examples/bookings/` so `zever.yaml` and `data/` resolve:

1. Migrate: `zever db migrate --adapter=sqlite --dsn=data/bookings.db schema/bookings.zen`
2. Seed (idempotent, safe to re-run): `go run ./db/seed`
   - Creates demo host (`host@example.com`) + guest (`guest@example.com`),
     two spaces, and one confirmed booking.
3. Worker (jobs + embedded scheduler, one process over the shared memory
   queue): `go run ./cmd/worker`
   - Registers `SendConfirmation` (queue `default`) and `SendReminder`
     (queue `low`), concurrency 4, graceful shutdown on SIGINT/SIGTERM.
   - Embedded scheduler fires the `Reminder` cron (`0 9 * * *`) dispatching
     `SendReminder`.
   - `SendConfirmation` loads the booking, sends a mail receipt
     (`mailer/log`) + guest push (`notification/log`), and delivers a
     `booking.created` webhook event (skipped quietly when nobody
     subscribes).
   - `SendReminder` notifies guests of confirmed bookings starting within
     7 days, notification only.

### Webhook demo targets

Two demo targets, `booking.created` and `booking.cancelled`, with
per-target secrets from the environment (secrets/env adapter, never stored
in `zever.yaml`):

- `BOOKINGS_WEBHOOK_CREATED_TARGET` / `BOOKINGS_WEBHOOK_CREATED_SECRET`
- `BOOKINGS_WEBHOOK_CANCELLED_TARGET` / `BOOKINGS_WEBHOOK_CANCELLED_SECRET`

Empty target URLs are skipped, so the worker runs without them. Targets
must be public HTTPS by default (`AllowPrivateTargets=false`); tests use
`httptest` servers with the `AllowPrivateTargets` option.
