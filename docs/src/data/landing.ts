/** Single source of truth for landing content. Counts are derived, never hard-coded. */

export const VERSION = 'v0.6.1';
export const INSTALL_WIN = `irm https://raw.githubusercontent.com/zenta-dev/zever/${VERSION}/install.ps1 | iex`;
export const INSTALL = `curl -fsSL https://raw.githubusercontent.com/zenta-dev/zever/${VERSION}/install.sh | sh`;

/** Adapter matrix mirrors config/README.md. */
export const SERVICES: Record<string, string[]> = {
  ai: ['anthropic', 'openai', 'gemini', 'ollama'],
  analytics: ['log', 'posthog'],
  auth: ['jwt', 'session', 'oidc'],
  billing: ['stub', 'stripe', 'paddle'],
  cache: ['memory', 'redis', 'db'],
  crypto: ['local'],
  db: ['sqlite', 'postgres'],
  document: ['local', 'remote', 'latex'],
  eventbus: ['memory', 'redis'],
  flag: ['static', 'firebase'],
  geo: ['google', 'static', 'osm'],
  i18n: ['embed', 'remote'],
  idempotency: ['memory', 'redis', 'db'],
  lock: ['memory', 'redis'],
  log: ['noop', 'zerolog', 'slog', 'pretty'],
  mailer: ['log', 'smtp'],
  media: ['local', 's3'],
  notification: ['log', 'twilio', 'fcm'],
  observability: ['noop', 'stdout', 'otlp'],
  outbox: ['memory', 'db', 'cdc'],
  password: ['argon2id'],
  payment: ['stub', 'stripe', 'paddle'],
  permission: ['noop', 'rbac', 'casbin'],
  queue: ['memory', 'redis', 'db'],
  ratelimit: ['memory', 'redis'],
  resilience: ['memory', 'redis'],
  router: ['fiber', 'stdhttp'],
  scheduler: ['embedded', 'postgres'],
  search: ['db', 'postgres', 'meilisearch', 'sqlite'],
  secrets: ['env', 'vault'],
  session: ['memory', 'redis', 'db'],
  storage: ['local', 's3', 'r2'],
  tenant: ['single', 'header'],
  vectorstore: ['db', 'sqlite', 'pgvector', 'qdrant'],
  webhook: ['http', 'queue'],
  workflow: ['memory', 'db', 'postgres'],
};

export const SERVICE_COUNT = Object.keys(SERVICES).length;
export const ADAPTER_COUNT = new Set(Object.values(SERVICES).flat()).size;

export type Category = 'Data' | 'Messaging' | 'Security' | 'Ops' | 'AI & Media';

export interface Battery {
  name: string;
  blurb: string;
  doc: string; // path under base, e.g. guides/data/cache
  category: Category;
  adapters: string[];
}

export const BATTERIES: Battery[] = [
  { name: 'Database & ORM', blurb: 'Typed generics query builder, migrations, seeding.', doc: 'guides/data/query-builder', category: 'Data', adapters: SERVICES.db },
  { name: 'Cache', blurb: 'One interface, memory to redis.', doc: 'guides/data/cache', category: 'Data', adapters: SERVICES.cache },
  { name: 'Search', blurb: 'Full-text across db, postgres, meilisearch.', doc: 'guides/data/search', category: 'Data', adapters: SERVICES.search },
  { name: 'Vector store', blurb: 'Embeddings on pgvector or qdrant.', doc: 'guides/data/vectorstore', category: 'Data', adapters: SERVICES.vectorstore },
  { name: 'Storage', blurb: 'Local, S3, R2 behind one API.', doc: 'guides/data/storage-media', category: 'Data', adapters: SERVICES.storage },
  { name: 'Queue', blurb: 'Durable jobs with visibility timeouts.', doc: 'guides/background/queue', category: 'Messaging', adapters: SERVICES.queue },
  { name: 'Scheduler & jobs', blurb: 'Cron on the same queue.', doc: 'guides/background/scheduler-jobs', category: 'Messaging', adapters: SERVICES.scheduler },
  { name: 'Event bus', blurb: 'In-process or redis pub/sub.', doc: 'guides/background/events', category: 'Messaging', adapters: SERVICES.eventbus },
  { name: 'Outbox', blurb: 'Transactional events, db or CDC.', doc: 'guides/background/outbox', category: 'Messaging', adapters: SERVICES.outbox },
  { name: 'Saga & workflows', blurb: 'Long-running flows with compensation.', doc: 'guides/background/saga', category: 'Messaging', adapters: SERVICES.workflow },
  { name: 'Mailer', blurb: 'Log in dev, SMTP in prod.', doc: 'guides/background/mailer', category: 'Messaging', adapters: SERVICES.mailer },
  { name: 'Notifications', blurb: 'SMS and push with one call.', doc: 'guides/background/notifications', category: 'Messaging', adapters: SERVICES.notification },
  { name: 'Webhooks', blurb: 'Signed delivery, queued retries.', doc: 'guides/background/webhooks', category: 'Messaging', adapters: SERVICES.webhook },
  { name: 'Auth', blurb: 'JWT, sessions, OIDC.', doc: 'guides/security/auth', category: 'Security', adapters: SERVICES.auth },
  { name: 'Permissions', blurb: 'RBAC and casbin policies.', doc: 'guides/security/authz-permissions', category: 'Security', adapters: SERVICES.permission },
  { name: 'Secrets', blurb: 'Env or vault, never logged.', doc: 'guides/security/secrets', category: 'Security', adapters: SERVICES.secrets },
  { name: 'Password hashing', blurb: 'argon2id defaults.', doc: 'guides/security/password-hashing', category: 'Security', adapters: SERVICES.password },
  { name: 'Rate limit', blurb: 'Token bucket, memory or redis.', doc: 'guides/security/ratelimit', category: 'Security', adapters: SERVICES.ratelimit },
  { name: 'Lock & idempotency', blurb: 'Safe retries across processes.', doc: 'guides/security/lock-idempotency', category: 'Security', adapters: SERVICES.lock },
  { name: 'Observability', blurb: 'Traces and metrics via OTLP.', doc: 'guides/operate/observability', category: 'Ops', adapters: SERVICES.observability },
  { name: 'Logging', blurb: 'zerolog, slog, pretty.', doc: 'guides/operate/logging', category: 'Ops', adapters: SERVICES.log },
  { name: 'Resilience', blurb: 'Breakers and bulkheads.', doc: 'guides/operate/resilience', category: 'Ops', adapters: SERVICES.resilience },
  { name: 'Feature flags', blurb: 'Static or firebase.', doc: 'guides/operate/flags', category: 'Ops', adapters: SERVICES.flag },
  { name: 'Tenants', blurb: 'Single or header-resolved.', doc: 'guides/operate/tenants', category: 'Ops', adapters: SERVICES.tenant },
  { name: 'Router', blurb: 'fiber or stdlib net/http.', doc: 'guides/http/routing', category: 'Ops', adapters: SERVICES.router },
  { name: 'AI', blurb: 'Anthropic, OpenAI, Gemini, Ollama.', doc: 'guides/integrations/ai', category: 'AI & Media', adapters: SERVICES.ai },
  { name: 'Media processing', blurb: 'Local or S3-backed pipelines.', doc: 'guides/data/media', category: 'AI & Media', adapters: SERVICES.media },
  { name: 'Documents', blurb: 'PDF and LaTeX rendering.', doc: 'guides/data/document', category: 'AI & Media', adapters: SERVICES.document },
  { name: 'Payments & billing', blurb: 'Stripe and Paddle.', doc: 'guides/integrations/payment', category: 'AI & Media', adapters: SERVICES.payment },
  { name: 'Geo', blurb: 'Geocoding, google or OSM.', doc: 'guides/integrations/geo', category: 'AI & Media', adapters: SERVICES.geo },
];

export const CATEGORIES: Category[] = ['Data', 'Messaging', 'Security', 'Ops', 'AI & Media'];

export interface SwapService {
  id: string;
  label: string;
  adapters: { id: string; config: string }[];
  call: string; // handler accessor snippet, constant per service
}

/** Playground presets: config changes, `call` never does. */
export const SWAP: SwapService[] = [
  {
    id: 'db', label: 'db', call: 'db, err := c.DB()',
    adapters: [
      { id: 'sqlite', config: 'db:\n  adapter: sqlite\n  options:\n    path: data/app.db' },
      { id: 'postgres', config: 'db:\n  adapter: postgres\n  options:\n    dsn: ${DB_DSN}\n    max_conns: 20' },
    ],
  },
  {
    id: 'cache', label: 'cache', call: 'cache, err := c.Cache()',
    adapters: [
      { id: 'memory', config: 'cache:\n  adapter: memory\n  options:\n    max_entries: 10000' },
      { id: 'redis', config: 'cache:\n  adapter: redis\n  options:\n    addr: ${REDIS_ADDR}' },
    ],
  },
  {
    id: 'queue', label: 'queue', call: 'q, err := c.Queue()',
    adapters: [
      { id: 'memory', config: 'queue:\n  adapter: memory' },
      { id: 'redis', config: 'queue:\n  adapter: redis\n  options:\n    addr: ${REDIS_ADDR}' },
      { id: 'db', config: 'queue:\n  adapter: db' },
    ],
  },
  {
    id: 'storage', label: 'storage', call: 'store, err := c.Storage()',
    adapters: [
      { id: 'local', config: 'storage:\n  adapter: local\n  options:\n    root: ./uploads' },
      { id: 's3', config: 'storage:\n  adapter: s3\n  options:\n    bucket: ${S3_BUCKET}' },
      { id: 'r2', config: 'storage:\n  adapter: r2\n  options:\n    bucket: ${R2_BUCKET}' },
    ],
  },
  {
    id: 'mailer', label: 'mailer', call: 'm, err := c.Mailer()',
    adapters: [
      { id: 'log', config: 'mailer:\n  adapter: log' },
      { id: 'smtp', config: 'mailer:\n  adapter: smtp\n  options:\n    host: ${SMTP_HOST}' },
    ],
  },
];

export const ZEN_SOURCE = `entity Task {
  id: uuid @primary
  title: string @validate(min_len: 1, max_len: 200)
  done: bool
  created_at: timestamp @default(now())
}

service TaskService {
  rpc CreateTask(title: string) -> Task {
    http: POST "/v1/tasks"
    auth: required
  }
  rpc ListTasks() -> Task {
    http: GET "/v1/tasks"
    auth: required
    paginated: true
  }
}`;

export const OUTPUTS = [
  { id: 'orm', label: 'Typed ORM', file: 'task.zenorm.go', hint: 'Generics table + column handles' },
  { id: 'openapi', label: 'OpenAPI', file: 'openapi.json', hint: 'Spec from the same schema' },
  { id: 'ddl', label: 'Migration DDL', file: 'schema.hcl', hint: 'Atlas schema from the same source' },
  { id: 'routes', label: 'Routes', file: 'routes.go', hint: 'POST /v1/tasks, GET /v1/tasks' },
] as const;

export const TERMINAL_STEPS = [
  { cmd: 'curl -fsSL https://raw.githubusercontent.com/zenta-dev/zever/' + VERSION + '/install.sh | sh', out: 'installed zever ' + VERSION },
  { cmd: 'zever new hello --dir ./hello && cd hello', out: 'scaffolded hello/ (sqlite, memory, log defaults)' },
  { cmd: 'zever check schema/app.zen', out: 'ok: 1 entity, 1 service, 2 rpcs' },
  { cmd: 'zever compile --backend=zenorm,atlas,openapi --out ./generated schema/app.zen', out: 'compiled 1 file(s) → ./generated' },
  { cmd: 'zever db migrate --adapter=sqlite --dsn=data/app.db schema/app.zen', out: 'applied 1 statement(s) via sqlite' },
];

export const EXAMPLES = [
  { name: 'showcase', tag: 'Full app', blurb: 'Largest end-to-end app: auth, queue, search, workers.' },
  { name: 'bookings', tag: 'Tutorial', blurb: 'The Booking API tutorial, runnable.' },
  { name: 'todo', tag: 'Starter', blurb: 'Smallest useful schema-to-API loop.' },
  { name: 'transactions', tag: 'ORM', blurb: 'Transactions and rollback patterns.' },
  { name: 'fts', tag: 'Search', blurb: 'Full-text search on sqlite and postgres.' },
  { name: 'pagination', tag: 'ORM', blurb: 'Cursor and offset pagination.' },
  { name: 'recursive_cte', tag: 'ORM', blurb: 'Recursive CTE queries, typed.' },
  { name: 'bulk_upsert', tag: 'ORM', blurb: 'Batch upserts without raw SQL.' },
  { name: 'json_query', tag: 'ORM', blurb: 'JSON column queries.' },
  { name: 'external-sms', tag: 'Adapters', blurb: 'Plug in a custom SMS adapter.' },
] as const;

export const STATS = [
  { value: 7.4, decimals: 1, suffix: 'x', label: 'faster queue reclaim sweep', repro: 'go -C adapters/queue/db test -run=NONE -bench=ReclaimStale -benchmem -count=10' },
  { value: 99.4, decimals: 1, suffix: '%', label: 'fewer allocs in closest matcher', repro: "go -C cmd/zever test -run=NONE -bench='^BenchmarkClosest$' -benchmem" },
  { value: 0, decimals: 0, suffix: '', label: 'allocs per mailer-log Send', repro: "go -C adapters/mailer/log test -run=NONE -bench='^BenchmarkSend$' -benchmem" },
  { value: 5, decimals: 0, suffix: 'x', label: 'faster config env overlay', repro: 'go -C config test -run=NONE -bench=ApplyEnv -benchmem -count=10' },
];

export const FAQ = [
  { q: 'Is Zever a web framework?', a: 'No. It compiles a schema into plumbing (ORM, specs, DDL, routing glue) and wires services through one container. Handlers and business logic stay yours.' },
  { q: 'Does it generate business logic?', a: 'Never. Only the infrastructure that logic runs on.' },
  { q: 'How do I move from sqlite to postgres?', a: 'Change `db.adapter` in config. Code calling `c.DB()` is untouched.' },
  { q: 'Do I need infrastructure to start?', a: 'No. Defaults use sqlite, memory, local, log and noop adapters, so the first build passes on a clean machine.' },
  { q: 'What happens to misconfiguration?', a: 'Config is strictly decoded: unknown services or fields fail at load, not at 3am.' },
  { q: 'Can AI agents drive it?', a: 'Yes. The CLI is agent-friendly, `generate` supports `--dry-run`, and there is an MCP codegen backend plus an agent skill.' },
  { q: 'Which Go version?', a: 'Go 1.27+.' },
  { q: 'License?', a: 'Apache-2.0.' },
];

export const NAV = [
  { label: 'Docs', href: 'start/introduction' },
  { label: 'Tutorials', href: 'tutorials/overview' },
  { label: 'Reference', href: 'reference/cli' },
  { label: 'Examples', href: 'tutorials/explore-the-showcase-app' },
];

export const FOOTER_COLS = [
  { title: 'Start', links: [['Installation', 'start/installation'], ['Configuration', 'guides/operate/configuration'], ['Directory structure', 'start/project-layout'], ['Deployment', 'guides/operate/deployment']] },
  { title: 'Learn', links: [['Booking API tutorial', 'tutorials/build-a-booking-api'], ['Schema DSL', 'reference/zen-syntax'], ['Container', 'concepts/container'], ['Query builder', 'guides/data/query-builder']] },
  { title: 'Reference', links: [['CLI', 'reference/cli'], ['Config', 'reference/config'], ['Adapters matrix', 'reference/adapters-matrix'], ['Agent skill', 'reference/agent-skill']] },
  { title: 'Project', links: [['Contribute', 'contribute'], ['Upgrade', 'guides/operate/upgrade']] },
];
