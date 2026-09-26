package resolver

import (
	"github.com/robfig/cron/v3"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// buildJobIndex flattens every module's already-resolved jobs into a single
// name -> *ir.Job map, needed by Pass 6 to resolve dispatch: targets. Names
// are globally unique across every declaration kind (Pass 0 guarantees
// this), so this is safe once Pass 5 has run.
func buildJobIndex(modules []*ir.Module) map[string]*ir.Job {
	idx := make(map[string]*ir.Job)

	for _, m := range modules {
		for _, j := range m.Jobs {
			idx[j.Name] = j
		}
	}

	return idx
}

// resolveSchedules is Pass 6's entry point: it resolves every schedule decl
// (in Pass 0's source-declaration order) and appends each resolved
// *ir.Schedule to its owning Module's Schedules slice, same reasoning as
// Pass 1's Module.Entities append.
func resolveSchedules(decls []*ast.ScheduleDecl, declModule map[ast.Decl]*ir.Module, jobsByName map[string]*ir.Job) diag.List {
	var diags diag.List

	for _, decl := range decls {
		module := declModule[decl]
		if module == nil {
			// Unreachable given Pass -1/0's invariants; guarded defensively.
			continue
		}

		sched, d := resolveSchedule(decl, module, jobsByName)
		diags = append(diags, d...)

		module.Schedules = append(module.Schedules, sched)
	}

	return diags
}

// resolveSchedule resolves one schedule's cron expression and dispatch
// target. No cross-module restriction is placed on Dispatch (deliberate,
// not an oversight): design doc §10's "no cross-module joins" is a
// database/entity-ownership rule, and dispatching a job is enqueuing onto a
// shared queue battery topic, not a DB access — a schedule in one module
// legitimately triggering a job declared in another is unremarkable, the
// same way any caller can already Dispatch() any registered job at the Go
// level today.
func resolveSchedule(decl *ast.ScheduleDecl, module *ir.Module, jobsByName map[string]*ir.Job) (*ir.Schedule, diag.List) {
	var diags diag.List

	if decl.Cron == "" {
		diags = append(diags, diag.Wrap("resolve", decl.Pos, ErrInvalidOption,
			"schedule %q is missing its required cron: option", decl.Name))
	} else if _, err := cron.ParseStandard(decl.Cron); err != nil {
		diags = append(diags, diag.Wrap("resolve", decl.CronPos, ErrInvalidCron, "invalid cron expression %q: %v", decl.Cron, err))
	}

	sched := &ir.Schedule{
		Name:       decl.Name,
		Cron:       decl.Cron,
		Module:     module,
		Pos:        decl.Pos,
		DocComment: decl.DocComment,
	}

	if decl.Dispatch != nil {
		job, args, d := resolveDispatch(decl.Dispatch, jobsByName)
		diags = append(diags, d...)
		sched.Dispatch = job
		sched.DispatchArgs = args
	}

	return sched, diags
}

// resolveDispatch resolves a schedule's dispatch: value. It must be a
// CallValue whose name resolves against the jobs map (missing or naming
// something that isn't a job -> ErrUnresolvedReference, same error either
// way since jobsByName only holds jobs); arity is checked
// len(Args) == len(job.Params) -> else ErrInvalidOption. Argument *type*
// checking against param types is not performed in v1 (arity only), same
// spirit as deferred RPC-param validation.
func resolveDispatch(v ast.Value, jobsByName map[string]*ir.Job) (*ir.Job, []any, diag.List) {
	call, ok := v.(*ast.CallValue)
	if !ok {
		return nil, nil, diag.List{diag.Wrap("resolve", valuePos(v), ErrUnresolvedReference,
			"dispatch must call a declared job, e.g. JobName(args...)")}
	}

	job, ok := jobsByName[call.Name]
	if !ok {
		return nil, nil, diag.List{diag.Wrap("resolve", call.Pos, ErrUnresolvedReference,
			"dispatch target %q is not a declared job", call.Name)}
	}

	var diags diag.List

	args := make([]any, 0, len(call.Args))
	for _, a := range call.Args {
		if nested, ok := a.Value.(*ast.CallValue); ok {
			diags = append(diags, diag.Wrap("resolve", valuePos(a.Value), ErrInvalidOption,
				"dispatch to job %s: argument %q is a nested call, which dispatch args cannot express (literal values only)",
				job.Name, nested.Name))

			continue
		}

		args = append(args, dispatchArgValue(a.Value))
	}

	if len(call.Args) != len(job.Params) {
		diags = append(diags, diag.Wrap("resolve", call.Pos, ErrInvalidOption,
			"dispatch to job %s passes %d argument(s), want %d", job.Name, len(call.Args), len(job.Params)))
	}

	return job, args, diags
}

// dispatchArgValue extracts a raw value from a dispatch argument's
// ast.Value for ir.Schedule.DispatchArgs. Argument type checking against
// the target job's declared param types is a v1 non-goal (arity only), so
// this is a best-effort literal extraction, not a validated decode.
// resolveDispatch's caller already rejects *ast.CallValue before reaching
// here (a nested call has no literal-shaped value dispatch args can carry),
// so this only ever sees the flat literal kinds.
func dispatchArgValue(v ast.Value) any {
	switch val := v.(type) {
	case *ast.StringLit:
		return val.Value
	case *ast.IntLit:
		return val.Value
	case *ast.FloatLit:
		return val.Value
	case *ast.DurationLit:
		return val.Value
	case *ast.IdentValue:
		return val.Name
	case *ast.SetLit:
		return val.Items
	default:
		return nil
	}
}
