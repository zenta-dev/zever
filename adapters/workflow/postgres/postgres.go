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

	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
)

// Run states persisted in the state column.
const (
	stateRunning   = "running"
	stateCompleted = "completed"
)

// Reclaimer is the crash-recovery extension over workflow.Workflow. Reclaim
// takes over runID's lease for owner when the recorded lease has expired
// and re-executes the run's step; a live lease held by another owner fails
// with ErrLeaseHeld.
type Reclaimer interface {
	// Reclaim takes over runID for owner and re-executes its step.
	Reclaim(ctx context.Context, runID workflow.RunID, owner string) error
}

// runRow is the workflow_runs entity. Column order matches runColumns:
// the positional Scan must read them in exactly this order.
type runRow struct {
	WorkflowID     string
	Step           string
	State          string
	Payload        []byte
	IdempotencyKey string
	LeaseOwner     string
	LeaseExpiresAt time.Time
	Attempt        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// runColumns is the entity column list in Scan order.
var runColumns = []string{
	"workflow_id", "step", "state", "payload", "idempotency_key",
	"lease_owner", "lease_expires_at", "attempt", "created_at", "updated_at",
}

// Scan reads one row positionally, coercing driver representations:
// timestamps arrive as time.Time (postgres) or RFC3339Nano text (sqlite),
// attempt as any integer width.
func (r *runRow) Scan(row orm.Row) error {
	var id, step, state, idem, owner string
	var payload []byte
	var expRaw, createdRaw, updatedRaw, attemptRaw any

	if err := row.Scan(&id, &step, &state, &payload, &idem, &owner,
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

	*r = runRow{
		WorkflowID: id, Step: step, State: state, Payload: payload,
		IdempotencyKey: idem, LeaseOwner: owner, LeaseExpiresAt: exp,
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

// driver is a DB-backed workflow.Workflow with lease-based crash recovery.
// It is safe for concurrent use; no lock is held while a step runs.
type driver struct {
	conn      coredb.DB
	tbl       orm.Table[runRow]
	cID       orm.Column[runRow, string]
	cStep     orm.Column[runRow, string]
	cState    orm.Column[runRow, string]
	cPayload  orm.Column[runRow, []byte]
	cIdem     orm.Column[runRow, string]
	cOwner    orm.Column[runRow, string]
	cExpires  orm.Column[runRow, time.Time]
	cAttempt  orm.Column[runRow, int64]
	cCreated  orm.Column[runRow, time.Time]
	cUpdated  orm.Column[runRow, time.Time]
	tableName string
	owner     string
	leaseTTL  time.Duration
	owns      bool
	mu        sync.RWMutex
	steps     map[string]workflow.StepFunc
	nextID    atomic.Uint64
}

var (
	_ workflow.Workflow      = (*driver)(nil)
	_ workflow.StepRegistrar = (*driver)(nil)
	_ Reclaimer              = (*driver)(nil)
)

// New creates a DB-backed workflow engine. Empty DSN selects sqlite at
// Path (default ":memory:"); a set DSN opens postgres. The runs table is
// created when missing. Note: ":memory:" sqlite uses shared cache, so two
// drivers with Path ":memory:" in one process share state; use distinct
// file paths for isolation.
func New(o Options) (workflow.Workflow, error) {
	return Open(o)
}

// Open creates a DB-backed workflow engine; see New.
func Open(o Options) (workflow.Workflow, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}

	poolOpts := o.Options
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

// NewFromDB creates a DB-backed workflow engine over an already-open
// coredb.DB, skipping DSN/Path construction. The caller retains ownership
// of db: Close on the returned Workflow does not close db, and a failed
// NewFromDB never closes db. Only sqlite and postgres dialects are
// supported; anything else fails closed.
func NewFromDB(conn coredb.DB, o Options) (workflow.Workflow, error) {
	if conn == nil {
		return nil, errors.New("postgres: db must not be nil")
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return openFromDB(conn, o, false)
}

// OpenFromDB creates a DB-backed workflow engine over an already-open
// coredb.DB; see NewFromDB.
func OpenFromDB(conn coredb.DB, o Options) (workflow.Workflow, error) {
	return NewFromDB(conn, o)
}

// openFromDB resolves table/owner/lease defaults, pings conn, ensures the
// schema, and wires the driver. owns reports whether the driver owns conn
// and may close it in Close.
func openFromDB(conn coredb.DB, o Options, owns bool) (workflow.Workflow, error) {
	table := o.Table
	if table == "" {
		table = DefaultTable
	}

	owner := o.Owner
	if owner == "" {
		owner = "workflow-owner"
	}

	ttl := o.LeaseTTL
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectTimeout)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}

	d := &driver{
		conn: conn, tbl: orm.NewTable[runRow](table, runColumns),
		cID:       orm.NewColumn[runRow, string](table, "workflow_id"),
		cStep:     orm.NewColumn[runRow, string](table, "step"),
		cState:    orm.NewColumn[runRow, string](table, "state"),
		cPayload:  orm.NewColumn[runRow, []byte](table, "payload"),
		cIdem:     orm.NewColumn[runRow, string](table, "idempotency_key"),
		cOwner:    orm.NewColumn[runRow, string](table, "lease_owner"),
		cExpires:  orm.NewColumn[runRow, time.Time](table, "lease_expires_at"),
		cAttempt:  orm.NewColumn[runRow, int64](table, "attempt"),
		cCreated:  orm.NewColumn[runRow, time.Time](table, "created_at"),
		cUpdated:  orm.NewColumn[runRow, time.Time](table, "updated_at"),
		tableName: table, owner: owner, leaseTTL: ttl, owns: owns,
		steps: make(map[string]workflow.StepFunc),
	}

	if err := d.ensureSchema(ctx); err != nil {
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

// ensureSchema creates the runs table when missing. DDL only: every
// state transition below goes through the orm typed builder.
func (d *driver) ensureSchema(ctx context.Context) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	quoted := `"` + strings.ReplaceAll(d.tableName, `"`, `""`) + `"`

	var ddl string

	if d.conn.Dialect() == "postgres" {
		ddl = `CREATE TABLE IF NOT EXISTS ` + quoted + ` (` +
			`workflow_id TEXT PRIMARY KEY, ` +
			`step TEXT NOT NULL, ` +
			`state TEXT NOT NULL, ` +
			`payload BYTEA NOT NULL, ` +
			`idempotency_key TEXT NOT NULL UNIQUE, ` +
			`lease_owner TEXT NOT NULL DEFAULT '', ` +
			`lease_expires_at TIMESTAMPTZ NOT NULL, ` +
			`attempt INTEGER NOT NULL DEFAULT 0, ` +
			`created_at TIMESTAMPTZ NOT NULL, ` +
			`updated_at TIMESTAMPTZ NOT NULL)`
	} else {
		ddl = `CREATE TABLE IF NOT EXISTS ` + quoted + ` (` +
			`workflow_id TEXT PRIMARY KEY, ` +
			`step TEXT NOT NULL, ` +
			`state TEXT NOT NULL, ` +
			`payload BLOB NOT NULL, ` +
			`idempotency_key TEXT NOT NULL UNIQUE, ` +
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

// load fetches one run row by id.
func (d *driver) load(ctx context.Context, id workflow.RunID) (*runRow, error) {
	row, ok, err := orm.From[runRow, *runRow](d.tbl).Where(d.cID.Eq(string(id))).First(ctx, d.conn)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, &workflow.UnknownRunError{RunID: id}
	}

	return row, nil
}

// RegisterStep registers fn under name, replacing any prior step.
func (d *driver) RegisterStep(name string, fn workflow.StepFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.steps == nil {
		d.steps = make(map[string]workflow.StepFunc)
	}

	d.steps[name] = fn
}

// stepFunc returns the step registered under name.
func (d *driver) stepFunc(name string) (workflow.StepFunc, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	fn, ok := d.steps[name]

	return fn, ok
}

// Start begins a new run: it inserts a running row holding this replica's
// lease, executes the step synchronously, then marks the run completed.
// A repeat Start with the same workflow ID returns DuplicateRunError
// without re-executing the step.
func (d *driver) Start(ctx context.Context, name string, input any, workflowID string) (workflow.RunID, error) {
	if err := d.checkDialect(); err != nil {
		return "", err
	}

	if workflowID != "" {
		if _, err := d.load(ctx, workflow.RunID(workflowID)); err == nil {
			return "", &workflow.DuplicateRunError{RunID: workflowID}
		} else if !errors.Is(err, workflow.ErrUnknownRun) {
			return "", err
		}
	}

	fn, ok := d.stepFunc(name)
	if !ok {
		return "", &workflow.UnknownStepError{Step: name}
	}

	id := workflow.RunID(workflowID)
	if id == "" {
		for {
			candidate := workflow.RunID(fmt.Sprintf("run-%d", d.nextID.Add(1)))

			_, err := d.load(ctx, candidate)
			if errors.Is(err, workflow.ErrUnknownRun) {
				id = candidate

				break
			}

			if err != nil {
				return "", err
			}
		}
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("postgres: start %q: marshal input: %w", name, err)
	}

	now := time.Now().UTC()

	err = orm.InsertInto(d.tbl).Values(
		orm.Set(d.cID, string(id)),
		orm.Set(d.cStep, name),
		orm.Set(d.cState, stateRunning),
		orm.Set(d.cPayload, inputJSON),
		orm.Set(d.cIdem, string(id)),
		orm.Set(d.cOwner, d.owner),
		orm.Set(d.cExpires, now.Add(d.leaseTTL)),
		orm.Set(d.cAttempt, int64(1)),
		orm.Set(d.cCreated, now),
		orm.Set(d.cUpdated, now),
	).Exec(ctx, d.conn)
	if err != nil {
		if isDuplicateErr(err) {
			return "", &workflow.DuplicateRunError{RunID: string(id)}
		}

		return "", err
	}

	result, err := fn(ctx, input)
	if err != nil {
		_, _ = orm.DeleteFrom(d.tbl).Where(d.cID.Eq(string(id))).Exec(ctx, d.conn)

		return "", fmt.Errorf("workflow: step %q failed: %w", name, err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		_, _ = orm.DeleteFrom(d.tbl).Where(d.cID.Eq(string(id))).Exec(ctx, d.conn)

		return "", fmt.Errorf("postgres: start %q: marshal result: %w", name, err)
	}

	// Owner-guarded so a stale Start finishing after a reclaim cannot
	// clobber the reclaimed result; zero rows means reclaimed.
	n, err := orm.UpdateTable(d.tbl).Where(orm.And(d.cID.Eq(string(id)), d.cOwner.Eq(d.owner))).Set(
		orm.Set(d.cPayload, resultJSON),
		orm.Set(d.cState, stateCompleted),
		orm.Set(d.cUpdated, time.Now().UTC()),
	).Exec(ctx, d.conn)
	if err != nil {
		return "", err
	}

	if n == 0 {
		return id, nil
	}

	return id, nil
}

// Signal records value as the run's latest payload while running;
// completed runs reject with RunCompletedError.
func (d *driver) Signal(ctx context.Context, runID workflow.RunID, _ string, value any) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	row, err := d.load(ctx, runID)
	if err != nil {
		return err
	}

	if row.State == stateCompleted {
		return &workflow.RunCompletedError{RunID: runID}
	}

	valueJSON, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("postgres: signal %q: marshal value: %w", runID, err)
	}

	// State-guarded so a Signal racing a completion cannot resurrect or
	// overwrite a completed run: zero rows means it finished or vanished.
	n, err := orm.UpdateTable(d.tbl).Where(orm.And(
		d.cID.Eq(string(runID)),
		d.cState.Eq(stateRunning),
	)).Set(
		orm.Set(d.cPayload, valueJSON),
		orm.Set(d.cUpdated, time.Now().UTC()),
	).Exec(ctx, d.conn)
	if err != nil {
		return err
	}
	if n == 0 {
		cur, rerr := d.load(ctx, runID)
		if rerr != nil {
			return rerr
		}
		if cur.State == stateCompleted {
			return &workflow.RunCompletedError{RunID: runID}
		}
		return workflow.ErrUnknownRun
	}

	return nil
}

// Query decodes the run's stored state into out.
func (d *driver) Query(ctx context.Context, runID workflow.RunID, name string, out any) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	row, err := d.load(ctx, runID)
	if err != nil {
		return err
	}

	if name != "state" {
		return &workflow.UnknownQueryError{Query: name}
	}

	if err := json.Unmarshal(row.Payload, out); err != nil {
		return fmt.Errorf("postgres: query %q: type mismatch: %w", name, err)
	}

	return nil
}

// Cancel removes a running run; completed runs reject with
// RunCompletedError and are kept for audit.
func (d *driver) Cancel(ctx context.Context, runID workflow.RunID) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	row, err := d.load(ctx, runID)
	if err != nil {
		return err
	}

	if row.State == stateCompleted {
		return &workflow.RunCompletedError{RunID: runID}
	}

	// State-guarded delete so a Cancel racing a completion keeps the
	// completed audit row; zero rows means it finished or vanished.
	n, err := orm.DeleteFrom(d.tbl).Where(orm.And(
		d.cID.Eq(string(runID)),
		d.cState.Eq(stateRunning),
	)).Exec(ctx, d.conn)
	if err != nil {
		return err
	}
	if n == 0 {
		cur, rerr := d.load(ctx, runID)
		if rerr != nil {
			return rerr
		}
		if cur.State == stateCompleted {
			return &workflow.RunCompletedError{RunID: runID}
		}
		return workflow.ErrUnknownRun
	}

	return nil
}

// Reclaim takes over runID for owner when its lease has expired and
// re-executes the run's step with the stored input. A live lease held by
// another owner fails with ErrLeaseHeld; completed runs fail with
// ErrRunCompleted. Steps must be idempotent: a reclaim re-executes them.
func (d *driver) Reclaim(ctx context.Context, runID workflow.RunID, owner string) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	if strings.TrimSpace(owner) == "" {
		return errors.New("postgres: reclaim requires a non-empty owner")
	}

	row, err := d.load(ctx, runID)
	if err != nil {
		return err
	}

	if row.State == stateCompleted {
		return &workflow.RunCompletedError{RunID: runID}
	}

	now := time.Now().UTC()
	if row.LeaseOwner != owner && row.LeaseExpiresAt.After(now) {
		return &LeaseHeldError{RunID: string(runID), Owner: row.LeaseOwner}
	}

	// Compare-and-set on lease expiry: only an expired lease (or our own)
	// transfers. Mirrors the reclaim.lua stale-claim shape.
	n, err := orm.UpdateTable(d.tbl).Where(orm.And(
		d.cID.Eq(string(runID)),
		orm.Or(d.cExpires.Lte(now), d.cOwner.Eq(owner)),
	)).Set(
		orm.Set(d.cOwner, owner),
		orm.Set(d.cExpires, now.Add(d.leaseTTL)),
		orm.Set(d.cAttempt, row.Attempt+1),
		orm.Set(d.cUpdated, now),
	).Exec(ctx, d.conn)
	if err != nil {
		return err
	}

	if n == 0 {
		cur, reloadErr := d.load(ctx, runID)
		if reloadErr != nil {
			return reloadErr
		}

		return &LeaseHeldError{RunID: string(runID), Owner: cur.LeaseOwner}
	}

	fn, ok := d.stepFunc(row.Step)
	if !ok {
		return &workflow.UnknownStepError{Step: row.Step}
	}

	var input any
	if len(row.Payload) > 0 {
		if uerr := json.Unmarshal(row.Payload, &input); uerr != nil {
			return fmt.Errorf("postgres: reclaim %q: decode input: %w", runID, uerr)
		}
	}

	result, err := fn(ctx, input)
	if err != nil {
		return fmt.Errorf("workflow: step %q failed: %w", row.Step, err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("postgres: reclaim %q: marshal result: %w", runID, err)
	}

	n, err = orm.UpdateTable(d.tbl).Where(orm.And(
		d.cID.Eq(string(runID)),
		d.cOwner.Eq(owner),
	)).Set(
		orm.Set(d.cPayload, resultJSON),
		orm.Set(d.cState, stateCompleted),
		orm.Set(d.cUpdated, time.Now().UTC()),
	).Exec(ctx, d.conn)
	if err != nil {
		return err
	}

	if n == 0 {
		return fmt.Errorf("postgres: reclaim %q: %w", runID, ErrLeaseHeld)
	}

	return nil
}

// Close releases the database pool when this driver owns its connection
// (built via New/Open). Drivers built via NewFromDB/OpenFromDB borrow the
// caller's DB and Close is a no-op.
func (d *driver) Close() error {
	if !d.owns {
		return nil
	}

	return d.conn.Close(context.Background())
}

// isDuplicateErr reports PRIMARY KEY / UNIQUE violations across dialects.
func isDuplicateErr(err error) bool {
	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "primary key")
}
