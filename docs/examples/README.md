# docs/examples: tested doc samples

Standalone Go module with Example-style tests mirroring the most-copied doc
samples. Deliberately excluded from go.work (like examples/external-sms), so
it proves real consumer resolution against published-shape requires plus
local replaces.

Covered samples: cache open/get/set (memory), db open (sqlite), auth
issue/verify (session adapter over a memory store), queue push/pop (memory),
minimal serve wiring (stdhttp router plus slog logger, handler only, no
network listen), plugin register plus resolve (container plugin with a stub
battery).

Run from the repo root:

GOWORK=off go -C docs/examples test ./...

CI runs the same command as a second step of the doc-snippets job.
