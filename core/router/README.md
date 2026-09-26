# router

Adapter registry for HTTP routers (`fiber`, `stdhttp`). Handlers are plain
`http.HandlerFunc`s; route params are exposed uniformly through
`router.Param` regardless of driver.

## Patterns

`Handle` and `Group.Handle` accept both `:name` and `{name}` param syntax on
the fiber driver; patterns are normalized to the driver-native form. The
`stdhttp` driver accepts only `net/http` ServeMux-native syntax.

| Pattern | fiber | stdhttp |
|---|---|---|
| `/users/:id` | `:id` | rejected (colon syntax) |
| `/users/{id}` | `:id` | `{id}` |
| `/files/{path...}` | `*` wildcard | `{path...}` subtree |
| `/users/{$}` | literal | `{$}` exact match |
| `/files/{name:[a-z]+}` | `:name<[a-z]+>` (driver-native) | rejected (regex constraint) |
| `/items/:id<[0-9]+>` | `:id<[0-9]+>` (driver-native) | rejected (regex constraint) |

Regex constraints differ per driver: fiber uses `:name<constraint>`, the
stdlib mux has no constraint syntax. The fiber driver passes
`:name<constraint>` through as-is and routes are matched case-sensitively,
so `/Users/:id` and `/users/:id` are distinct routes; duplicate detection is
likewise case-sensitive.

## Groups

`Group(prefix, middlewares...)` and `Group.Group` apply the given
`http.Handler` middlewares to the group's routes only (non-group routes are
unaffected), in order, for both drivers. `Use` applies middlewares
router-wide or group-wide respectively. On `stdhttp`, `Use` affects only
routes registered after the call (middleware is snapshotted per route).

## Params

Drivers inject matched route params into the request context under a uniform
key:

```go
r.Handle("GET", "/users/{id}", func(w http.ResponseWriter, req *http.Request) {
    id := router.Param(req, "id")
})
```

- `router.Param(r *http.Request, name string) string` — single param by name; empty when absent.
- `router.ParamNames(r *http.Request) map[string]string` — all params as a copy.
- `router.WithParams(ctx, params)` — context helper for tests and middleware; used by the drivers.

## Methods

Method strings are validated against the standard set (GET, POST, PUT, PATCH,
DELETE, HEAD, OPTIONS, CONNECT, TRACE). Invalid or duplicate registrations
are skipped with a `[router] <driver>:` log line: the fiber driver leaves the
route unregistered; the stdhttp driver relies on the mux's automatic 405
Method Not Allowed for registered paths and skips conflicting patterns.
