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

1. Migrate: `zever db migrate --adapter=sqlite --dsn=data/app.db schema/bookings.zen`
2. Seed: coming in a later PR.
3. Serve: coming in a later PR (`zever serve --config zever.yaml`).
4. Worker: coming in a later PR (queue `default`/`low` consumers for
   SendConfirmation/SendReminder + embedded scheduler for Reminder cron).

## Doctor note

`zever doctor --config zever.yaml` reports FAILs that are expected with
zero-infra defaults, not framework bugs (do not fix framework code):

- ai, auth, geo, i18n: same 4 FAILs as bare defaults (missing api key,
  jwt secret, cities.json, embed FS).
- db: `data/app.db` resolves relative to `examples/bookings/`; run doctor
  from that directory or after migrating.
- webhook (queue variant): needs its queue reference, wired in a later PR.
