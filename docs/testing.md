# testing

Rules that keep tests deterministic and flake-free. Flaky tests erode trust
in the suite and in CI; these rules prevent the classes of flakes we have
already fixed so they do not come back. For the error-path and boundary
checklist used when auditing a module, see [`edge-cases.md`](edge-cases.md).

## Determinism

- No `time.Sleep` for synchronization. Wait on channels, `sync` primitives,
  or a `waitFor` polling helper that asserts a condition with a deadline.
- No network calls. Tests run offline; use stubs or in-process fakes.
- No unseeded randomness. Seed explicitly (`rand.New(rand.NewSource(1))`) or
  use fixed fixtures.
- No timing assertions. Never assert on wall-clock duration; assert on
  behavior (did the timeout fire, did the deadline cancel the context).

## Process-global state

`go test -count=N` reruns every test in one process, so state that outlives
a single run leaks into the next.

- Never register fixed names into package-global registries in tests:
  `Register`, `RegisterShared`, `RegisterPluginValidator`, `dialect.Register`,
  `X.Register(X.Adapter("..."))`. The second run collides with the first
  run's registration.
- Use per-invocation unique names (e.g. `fmt.Sprintf("stub-%d", n)`),
  `sync.OnceValue` for one-time fixtures, or instance-local registries that
  the test constructs and tears down itself.

## Global os.Stdout capture

Swapping `os.Stdout` replaces a process-global handle. Under `t.Parallel()`
that races with every other test's output and with the test framework's own
writer.

- Never capture `os.Stdout` in a test that also calls `t.Parallel()`.
- Prefer injecting an `io.Writer` (function parameter, struct field,
  `log.SetOutput` on a logger the test owns) over redirecting the global.

## Shared DB state

- `:memory:` sqlite is per-connection unless the DSN sets `cache=shared`;
  with `cache=shared` every test sees the same tables. Do not rely on
  either behavior across tests.
- Give each test its own file under `t.TempDir()` (or its own uniquely
  named schema) so runs are independent and `-shuffle=on` is safe.

## Time-dependent code

- Prefer `testing/synctest` bubbles for timeout and deadline assertions:
  the bubble's fake clock makes "wait 500ms then assert" instant and
  deterministic.
- If `synctest` does not fit, keep timeouts short and assert only on
  observable behavior, never on elapsed wall-clock time.

## fork/exec races

- `ETXTBSY` (golang/go#22315): a binary that was just written can fail to
  exec because the file is still busy. This bites tests that build and run
  a stub binary in a loop.
- Use a bounded retry around the exec, or guard the exec with
  `syscall.ForkLock` (`syscall.ForkLock.RLock()` around `exec.Cmd.Run`).

## Reproducing flakes

Run the suspect test hard before calling it fixed:

```sh
go test -run '^TestName$' -race -count=100 -failfast -shuffle=on ./...
make test-flake FLAKE_COUNT=5
```

`make test-flake` runs every module under the race detector with shuffled
order and repeated iterations. The nightly `flake-hunt` workflow
([`.github/workflows/flake.yml`](../.github/workflows/flake.yml)) does the
same across 8 shards; a red run there names a test to fix, it never blocks
unrelated merges.
