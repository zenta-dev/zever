package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"

	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/adapters/log/noop"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/shared/codec"
)

var argsCodec = codec.JSONCodec[any]{}

// slotRow is the scheduler_slots entity. Column order matches slotColumns:
// the positional Scan must read them in exactly this order.
type slotRow struct {
	Slot           string
	Spec           string
	JobName        string
	Args           []byte
	LeaseOwner     string
	LeaseExpiresAt time.Time
	Attempt        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// slotColumns is the entity column list in Scan order.
var slotColumns = []string{
	"slot", "spec", "job_name", "args",
	"lease_owner", "lease_expires_at", "attempt", "created_at", "updated_at",
}

// Scan reads one row positionally, coercing driver representations:
// timestamps arrive as time.Time (postgres) or RFC3339Nano text (sqlite),
// attempt as any integer width.
func (r *slotRow) Scan(row orm.Row) error {
	var slot, spec, jobName, owner string

	var args []byte

	var expRaw, createdRaw, updatedRaw, attemptRaw any

	if err := row.Scan(&slot, &spec, &jobName, &args, &owner,
		&expRaw, &attemptRaw, &createdRaw, &updatedRaw); err != nil {
		return err
	}

	exp, err := coerceTime(expRaw)
	if err != nil {
		return fmt.Errorf("postgres: scan lease_expires_at: %w", err)
	}

	created, err := coerceTime(createdRaw)
	if err != nil {
		return fmt.Errorf("postgres: scan created_at: %w", err)
	}

	updated, err := coerceTime(updatedRaw)
	if err != nil {
		return fmt.Errorf("postgres: scan updated_at: %w", err)
	}

	attempt, err := coerceInt(attemptRaw)
	if err != nil {
		return fmt.Errorf("postgres: scan attempt: %w", err)
	}

	*r = slotRow{
		Slot: slot, Spec: spec, JobName: jobName, Args: args,
		LeaseOwner: owner, LeaseExpiresAt: exp,
		Attempt: attempt, CreatedAt: created, UpdatedAt: updated,
	}

	return nil
}

// coerceTime converts a timestamp cell to time.Time.
func coerceTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return t, nil
	case string:
		return time.Parse(time.RFC3339Nano, t)
	case []byte:
		return time.Parse(time.RFC3339Nano, string(t))
	default:
		return time.Time{}, fmt.Errorf("postgres: unsupported timestamp %T", v)
	}
}

// coerceInt converts an integer cell to int64.
func coerceInt(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int32:
		return int64(n), nil
	case int:
		return int64(n), nil
	case float64:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("postgres: unsupported integer %T", v)
	}
}

// driver is a DB-leased scheduler.Scheduler. Cron ticks stay in-process
// (robfig/cron like the embedded adapter) but every tick claims its slot
// row in the database before dispatching, so at most one live owner fires
// a slot. It is safe for concurrent use.
type driver struct {
	conn         coredb.DB
	tbl          orm.Table[slotRow]
	cSlot        orm.Column[slotRow, string]
	cSpec        orm.Column[slotRow, string]
	cJob         orm.Column[slotRow, string]
	cArgs        orm.Column[slotRow, []byte]
	cOwner       orm.Column[slotRow, string]
	cExpires     orm.Column[slotRow, time.Time]
	cAttempt     orm.Column[slotRow, int64]
	cCreated     orm.Column[slotRow, time.Time]
	cUpdated     orm.Column[slotRow, time.Time]
	tableName    string
	owner        string
	leaseTTL     time.Duration
	dispatcher   *job.Dispatcher
	closeTimeout time.Duration
	logger       log.Logger
	owns         bool

	mu      sync.Mutex
	cron    *cron.Cron
	slots   map[scheduler.EntryID]string
	args    map[string]any
	nextID  atomic.Uint64
	started bool
}

var _ scheduler.Scheduler = (*driver)(nil)

// New creates a DB-leased scheduler. Empty DSN selects sqlite at Path
// (default ":memory:"); a set DSN opens postgres. The slots table is
// created when missing and claimable rows are adopted. Note: ":memory:"
// sqlite uses shared cache, so two drivers with Path ":memory:" in one
// process share state; use distinct file paths for isolation.
func New(o Options) (scheduler.Scheduler, error) {
	return Open(o)
}

// Open creates a DB-leased scheduler; see New.
func Open(o Options) (scheduler.Scheduler, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}

	poolOpts := o.PoolOptions
	if strings.TrimSpace(poolOpts.DSN) == "" && poolOpts.Path == "" {
		poolOpts.Path = ":memory:"
	}

	var (
		conn coredb.DB
		err  error
	)

	if strings.TrimSpace(poolOpts.DSN) != "" {
		conn, err = dbpostgres.New(poolOpts)
	} else {
		conn, err = dbsqlite.New(poolOpts)
	}

	if err != nil {
		return nil, err
	}

	d, err := openFromDB(conn, o, true)
	if err != nil {
		_ = conn.Close(context.Background())

		return nil, err
	}

	return d, nil
}

// NewFromDB creates a DB-leased scheduler over an already-open coredb.DB,
// skipping DSN/Path construction. The caller retains ownership of db:
// Close on the returned Scheduler does not close db, and a failed
// NewFromDB never closes db. Only sqlite and postgres dialects are
// supported; anything else fails closed.
func NewFromDB(conn coredb.DB, o Options) (scheduler.Scheduler, error) {
	if conn == nil {
		return nil, errors.New("postgres: db must not be nil")
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return openFromDB(conn, o, false)
}

// OpenFromDB creates a DB-leased scheduler over an already-open coredb.DB;
// see NewFromDB.
func OpenFromDB(conn coredb.DB, o Options) (scheduler.Scheduler, error) {
	return NewFromDB(conn, o)
}

// openFromDB resolves table/owner/lease defaults, pings conn, ensures the
// schema, adopts claimable slots, and wires the driver. owns reports
// whether the driver owns conn and may close it in Close.
func openFromDB(conn coredb.DB, o Options, owns bool) (scheduler.Scheduler, error) {
	table := o.Table
	if table == "" {
		table = DefaultTable
	}

	owner := o.Owner
	if owner == "" {
		owner = "scheduler-owner"
	}

	ttl := o.LeaseTTL
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}

	logger := o.Logger
	if logger == nil {
		logger = noop.New()
	}

	closeTimeout := o.CloseTimeout
	if closeTimeout == 0 {
		closeTimeout = scheduler.DefaultCloseTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectTimeout)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}

	d := &driver{
		conn: conn, tbl: orm.NewTable[slotRow](table, slotColumns),
		cSlot:     orm.NewColumn[slotRow, string](table, "slot"),
		cSpec:     orm.NewColumn[slotRow, string](table, "spec"),
		cJob:      orm.NewColumn[slotRow, string](table, "job_name"),
		cArgs:     orm.NewColumn[slotRow, []byte](table, "args"),
		cOwner:    orm.NewColumn[slotRow, string](table, "lease_owner"),
		cExpires:  orm.NewColumn[slotRow, time.Time](table, "lease_expires_at"),
		cAttempt:  orm.NewColumn[slotRow, int64](table, "attempt"),
		cCreated:  orm.NewColumn[slotRow, time.Time](table, "created_at"),
		cUpdated:  orm.NewColumn[slotRow, time.Time](table, "updated_at"),
		tableName: table, owner: owner, leaseTTL: ttl,
		dispatcher: o.Dispatcher, closeTimeout: closeTimeout, logger: logger,
		owns: owns, cron: cron.New(),
		slots: make(map[scheduler.EntryID]string),
		args:  make(map[string]any),
	}

	if err := d.ensureSchema(ctx); err != nil {
		return nil, err
	}

	if err := d.adopt(ctx); err != nil {
		return nil, err
	}

	return d, nil
}

// checkDialect fails closed on dialects outside sqlite/postgres.
func (d *driver) checkDialect() error {
	switch d.conn.Dialect() {
	case "sqlite", "postgres":
		return nil
	default:
		return fmt.Errorf("orm: postgres: unsupported dialect %q: %w",
			d.conn.Dialect(), dialect.ErrUnsupportedByDialect)
	}
}

// ensureSchema creates the slots table when missing. DDL only: every
// state transition below goes through the orm typed builder.
func (d *driver) ensureSchema(ctx context.Context) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	quoted := `"` + strings.ReplaceAll(d.tableName, `"`, `""`) + `"`

	var ddl string

	if d.conn.Dialect() == "postgres" {
		ddl = `CREATE TABLE IF NOT EXISTS ` + quoted + ` (` +
			`slot TEXT PRIMARY KEY, ` +
			`spec TEXT NOT NULL, ` +
			`job_name TEXT NOT NULL, ` +
			`args BYTEA NOT NULL, ` +
			`lease_owner TEXT NOT NULL DEFAULT '', ` +
			`lease_expires_at TIMESTAMPTZ NOT NULL, ` +
			`attempt INTEGER NOT NULL DEFAULT 0, ` +
			`created_at TIMESTAMPTZ NOT NULL, ` +
			`updated_at TIMESTAMPTZ NOT NULL)`
	} else {
		ddl = `CREATE TABLE IF NOT EXISTS ` + quoted + ` (` +
			`slot TEXT PRIMARY KEY, ` +
			`spec TEXT NOT NULL, ` +
			`job_name TEXT NOT NULL, ` +
			`args BLOB NOT NULL, ` +
			`lease_owner TEXT NOT NULL DEFAULT '', ` +
			`lease_expires_at TEXT NOT NULL, ` +
			`attempt INTEGER NOT NULL DEFAULT 0, ` +
			`created_at TEXT NOT NULL, ` +
			`updated_at TEXT NOT NULL)`
	}

	if _, err := d.conn.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("postgres: ensure schema: %w", err)
	}

	return nil
}

// adopt claims every slot row whose lease is expired or already held by
// this owner and registers a local cron entry for it. Rows leased to a
// live foreign owner are skipped: that instance keeps firing them. This
// is the crash-recovery path: a dead instance's expired leases resume
// here without any clock coordination beyond lease expiry.
func (d *driver) adopt(ctx context.Context) error {
	rows, err := orm.From[slotRow, *slotRow](d.tbl).All(ctx, d.conn)
	if err != nil {
		return err
	}

	for _, row := range rows {
		if err := d.adoptRow(ctx, row); err != nil {
			return err
		}
	}

	return nil
}

// adoptRow claims row when its lease is free and registers its tick.
func (d *driver) adoptRow(ctx context.Context, row *slotRow) error {
	now := time.Now().UTC()
	if row.LeaseOwner != d.owner && row.LeaseExpiresAt.After(now) {
		return nil
	}

	claimed, err := d.claim(ctx, row.Slot, now)
	if err != nil || !claimed {
		return err
	}

	var args any
	if len(row.Args) > 0 {
		if err := json.Unmarshal(row.Args, &args); err != nil {
			return fmt.Errorf("postgres: adopt %q: decode args: %w", row.Slot, err)
		}
	}

	if err := d.register(row.Spec, row.JobName, args, row.Slot); err != nil {
		return err
	}

	return nil
}

// claim transfers slot to this owner when its lease is expired or already
// ours, bumping attempt only on takeover from another owner. It reports
// whether the transfer landed; a lost race reports false with no error.
func (d *driver) claim(ctx context.Context, slot string, now time.Time) (bool, error) {
	n, err := orm.UpdateTable(d.tbl).Where(orm.And(
		d.cSlot.Eq(slot),
		orm.Or(d.cExpires.Lte(now), d.cOwner.Eq(d.owner)),
	)).Set(
		orm.Set(d.cOwner, d.owner),
		orm.Set(d.cExpires, now.Add(d.leaseTTL)),
		orm.Set(d.cUpdated, now),
	).Exec(ctx, d.conn)
	if err != nil {
		return false, err
	}

	return n == 1, nil
}

// load fetches one slot row by id, reporting absence as ok=false.
func (d *driver) load(ctx context.Context, slot string) (row *slotRow, ok bool, err error) {
	row, ok, err = orm.From[slotRow, *slotRow](d.tbl).Where(d.cSlot.Eq(slot)).First(ctx, d.conn)
	if err != nil {
		return nil, false, err
	}

	return row, ok, nil
}

// register parses spec and adds the gated tick for slot to the local cron.
// Callers must hold the lease (or own the fresh row) before registering.
func (d *driver) register(spec, jobName string, args any, slot string) error {
	parsed, err := cron.ParseStandard(spec)
	if err != nil {
		return &scheduler.InvalidSpecError{Spec: spec, Err: err}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	id := d.cron.Schedule(parsed, cron.FuncJob(func() {
		d.fire(slot)
	}))

	//nolint:gosec // cron EntryIDs are a small positive sequence starting at 1.
	entry := scheduler.EntryID(id)
	d.slots[entry] = slot
	d.args[slot] = args

	return nil
}

// fire claims slot and dispatches its job when the claim lands. A live
// foreign lease means another instance owns the slot: the tick is skipped,
// which is what makes multi-instance firing safe.
func (d *driver) fire(slot string) {
	ctx := context.Background()

	row, ok, err := d.load(ctx, slot)
	if err != nil || !ok {
		return
	}

	now := time.Now().UTC()
	if row.LeaseOwner != d.owner && row.LeaseExpiresAt.After(now) {
		return
	}

	claimed, err := d.claim(ctx, slot, now)
	if err != nil || !claimed {
		return
	}

	d.mu.Lock()
	args := d.args[slot]
	d.mu.Unlock()

	if args == nil && len(row.Args) > 0 {
		var decoded any
		if err := json.Unmarshal(row.Args, &decoded); err != nil {
			d.logger.Warn().Str("slot", slot).Err(err).Msg("postgres: decode args")

			return
		}

		args = decoded
	}

	// Each fire runs detached from any registration trace, mirroring the
	// embedded adapter: Dispatch roots its own schedule.<jobName> span.
	if err := d.dispatcher.Dispatch(ctx, row.JobName, args); err != nil {
		d.logger.Warn().Str("slot", slot).Str("job", row.JobName).Err(err).Msg("postgres: dispatch")
	}
}

// Schedule validates spec, jobName, and args, persists the slot row holding
// this replica's lease, and registers the local tick. Unknown job names
// wrap job.ErrUnknownJob.
func (d *driver) Schedule(ctx context.Context, spec, jobName string, args any) (scheduler.EntryID, error) {
	if err := d.checkDialect(); err != nil {
		return 0, err
	}

	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("scheduler: schedule %q: %w", spec, err)
	}

	if len(spec) == 0 || len(spec) > scheduler.MaxSpecLen {
		return 0, &scheduler.InvalidSpecError{Spec: spec, Err: errors.New("spec length must be 1-256")}
	}

	if _, err := cron.ParseStandard(spec); err != nil {
		return 0, &scheduler.InvalidSpecError{Spec: spec, Err: err}
	}

	if _, ok := job.Lookup(jobName); !ok {
		return 0, fmt.Errorf("scheduler: unknown job %q: %w", jobName, job.ErrUnknownJob)
	}

	argsJSON, err := argsCodec.Encode(args)
	if err != nil {
		return 0, fmt.Errorf("scheduler: args: %w", err)
	}

	now := time.Now().UTC()

	var slot string

	for {
		candidate := fmt.Sprintf("sched-%d", d.nextID.Add(1))

		_, ok, err := d.load(ctx, candidate)
		if err != nil {
			return 0, err
		}

		if !ok {
			slot = candidate

			break
		}
	}

	err = orm.InsertInto(d.tbl).Values(
		orm.Set(d.cSlot, slot),
		orm.Set(d.cSpec, spec),
		orm.Set(d.cJob, jobName),
		orm.Set(d.cArgs, argsJSON),
		orm.Set(d.cOwner, d.owner),
		orm.Set(d.cExpires, now.Add(d.leaseTTL)),
		orm.Set(d.cAttempt, int64(1)),
		orm.Set(d.cCreated, now),
		orm.Set(d.cUpdated, now),
	).Exec(ctx, d.conn)
	if err != nil {
		return 0, err
	}

	if err := d.register(spec, jobName, args, slot); err != nil {
		_, _ = orm.DeleteFrom(d.tbl).Where(d.cSlot.Eq(slot)).Exec(ctx, d.conn)

		return 0, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for entry, s := range d.slots {
		if s == slot {
			return entry, nil
		}
	}

	return 0, errors.New("postgres: schedule registered no entry")
}

// Remove unregisters the schedule with the given ID and deletes its slot
// row. Zero is invalid and fails; an unknown ID is a no-op returning nil.
func (d *driver) Remove(id scheduler.EntryID) error {
	if id == 0 {
		return &scheduler.InvalidOptionsError{Reason: "invalid_entry_id"}
	}

	d.mu.Lock()
	slot, ok := d.slots[id]
	if ok {
		delete(d.slots, id)
		delete(d.args, slot)
	}
	d.mu.Unlock()

	//nolint:gosec // IDs originate from Schedule, a small positive cron sequence.
	d.cron.Remove(cron.EntryID(id))

	if ok {
		_, _ = orm.DeleteFrom(d.tbl).Where(d.cSlot.Eq(slot)).Exec(context.Background(), d.conn)
	}

	return nil
}

// Entries returns a snapshot of the live entry IDs.
func (d *driver) Entries() []scheduler.EntryID {
	d.mu.Lock()
	defer d.mu.Unlock()

	ids := make([]scheduler.EntryID, 0, len(d.slots))
	for id := range d.slots {
		ids = append(ids, id)
	}

	return ids
}

// Start begins cron ticks. It is idempotent and non-blocking.
func (d *driver) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started {
		return nil
	}

	d.cron.Start()
	d.started = true

	return nil
}

// Stop ends cron ticks, waiting for running ticks up to CloseTimeout.
// Stopping a scheduler that was never started is a no-op returning nil.
// Leased rows are left in place so a restart or a peer reclaims them.
func (d *driver) Stop() error {
	d.mu.Lock()

	if !d.started {
		d.mu.Unlock()

		return nil
	}

	done := d.cron.Stop()
	timeout := d.closeTimeout
	d.mu.Unlock()

	select {
	case <-done.Done():
		d.mu.Lock()
		d.started = false
		d.mu.Unlock()

		return nil
	case <-time.After(timeout):
		return fmt.Errorf("scheduler: stop timeout: %w", context.DeadlineExceeded)
	}
}

// Close stops ticks and releases the database pool when this driver owns
// its connection (built via New/Open). Drivers built via NewFromDB/
// OpenFromDB borrow the caller's DB and Close skips the pool close but
// still stops ticks. The Scheduler interface needs only Stop; Close is
// the resource complement for process shutdown.
func (d *driver) Close() error {
	stopErr := d.Stop()

	if !d.owns {
		return stopErr
	}

	return errors.Join(stopErr, d.conn.Close(context.Background()))
}

// Name returns the adapter name for the scheduler.
func (d *driver) Name() string { return string(Adapter) }
