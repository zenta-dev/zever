package resolver

import (
	"time"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// resolveJobs is Pass 5's entry point: it resolves every job decl (in Pass
// 0's source-declaration order) and appends each resolved *ir.Job to its
// owning Module's Jobs slice, same reasoning as Pass 1's Module.Entities
// append.
func resolveJobs(
	decls []*ast.JobDecl, declModule map[ast.Decl]*ir.Module, entityByName map[string]*ir.Entity,
	messageByName map[string]*ir.Message, enumByName map[string]*ir.Enum,
) diag.List {
	var diags diag.List

	for _, decl := range decls {
		module := declModule[decl]
		if module == nil {
			// Unreachable given Pass -1/0's invariants; guarded defensively.
			continue
		}

		job, d := resolveJob(decl, module, entityByName, messageByName, enumByName)
		diags = append(diags, d...)

		module.Jobs = append(module.Jobs, job)
	}

	return diags
}

// resolveJob resolves one job's params, queue (defaulting "default", not
// validated against actually-registered queue adapters — a runtime/
// container concern with no compile-time visibility, explicit non-goal),
// and retry policy.
func resolveJob(
	decl *ast.JobDecl, module *ir.Module, entityByName map[string]*ir.Entity, messageByName map[string]*ir.Message, enumByName map[string]*ir.Enum,
) (*ir.Job, diag.List) {
	params, d := resolveParams(decl.Params, entityByName, messageByName, enumByName)
	diags := make(diag.List, 0, len(d))
	diags = append(diags, d...)

	queue := decl.Queue
	if queue == "" {
		queue = "default"
	}

	retry, d2 := resolveRetryPolicy(decl.Retry)
	diags = append(diags, d2...)

	job := &ir.Job{
		Name:       decl.Name,
		Module:     module,
		Params:     params,
		Queue:      queue,
		Retry:      retry,
		Pos:        decl.Pos,
		DocComment: decl.DocComment,
	}

	return job, diags
}

// resolveRetryPolicy decodes a job's retry: list. Each element must be
// max_attempts(<int > 0>) or backoff(exponential, base: <duration>) (only
// backoff kind in v1, matching job.RetryPolicy's doubling semantics).
//
// Absent retry: -> RetryPolicy{MaxAttempts: 1} is deliberately NOT the job
// battery's own runtime default of 25 attempts (job/job.go:29-31):
// compile-time "unspecified" and runtime default are different concepts,
// and conflating them would bake a battery-specific number into schema
// files that have nothing to do with that battery's implementation.
func resolveRetryPolicy(vals []ast.Value) (ir.RetryPolicy, diag.List) {
	policy := ir.RetryPolicy{MaxAttempts: 1}

	if len(vals) == 0 {
		return policy, nil
	}

	policy.Pos = valuePos(vals[0])

	var diags diag.List

	for _, v := range vals {
		call, ok := v.(*ast.CallValue)
		if !ok {
			diags = append(diags, diag.Wrap("resolve", valuePos(v), ErrInvalidOption,
				"retry entry must be max_attempts(<int>) or backoff(exponential, base: <duration>)"))

			continue
		}

		switch call.Name {
		case "max_attempts":
			n, d := decodeMaxAttempts(call)
			if d != nil {
				diags = append(diags, d)
				continue
			}

			policy.MaxAttempts = n
		case "backoff":
			base, kind, d := decodeBackoff(call)
			if d != nil {
				diags = append(diags, d)
				continue
			}

			policy.Backoff = kind
			policy.Base = base
		default:
			diags = append(diags, diag.Wrap("resolve", call.Pos, ErrInvalidOption, "unknown retry call %q", call.Name))
		}
	}

	return policy, diags
}

// decodeMaxAttempts decodes max_attempts(<int > 0>)'s single positional
// integer argument.
func decodeMaxAttempts(call *ast.CallValue) (int, *diag.Diagnostic) {
	if len(call.Args) != 1 || call.Args[0].Name != "" {
		return 0, diag.Wrap("resolve", call.Pos, ErrInvalidOption, "max_attempts requires exactly one positional integer argument")
	}

	lit, ok := call.Args[0].Value.(*ast.IntLit)
	if !ok {
		return 0, diag.Wrap("resolve", call.Args[0].Pos, ErrInvalidOption, "max_attempts argument must be an integer")
	}

	if lit.Value <= 0 {
		return 0, diag.Wrap("resolve", lit.Pos, ErrInvalidOption, "max_attempts must be greater than zero")
	}

	return int(lit.Value), nil
}

// decodeBackoff decodes backoff(exponential, base: <duration>)'s two
// arguments: a positional "exponential" identifier (the only backoff kind
// supported in v1) and a named base duration.
func decodeBackoff(call *ast.CallValue) (time.Duration, string, *diag.Diagnostic) {
	var kind string

	var base time.Duration

	haveKind, haveBase := false, false

	for i, arg := range call.Args {
		if arg.Name == "" {
			if i != 0 {
				return 0, "", diag.Wrap("resolve", arg.Pos, ErrInvalidOption, "backoff takes exactly one positional argument")
			}

			ident, ok := arg.Value.(*ast.IdentValue)
			if !ok || ident.Name != "exponential" {
				return 0, "", diag.Wrap("resolve", arg.Pos, ErrInvalidOption,
					"backoff kind must be exponential (only kind supported in v1)")
			}

			kind, haveKind = ident.Name, true

			continue
		}

		if arg.Name != "base" {
			return 0, "", diag.Wrap("resolve", arg.Pos, ErrInvalidOption, "unknown backoff argument %q", arg.Name)
		}

		dur, ok := arg.Value.(*ast.DurationLit)
		if !ok || !dur.Valid {
			return 0, "", diag.Wrap("resolve", arg.Pos, ErrInvalidOption, "backoff base must be a valid duration")
		}

		base, haveBase = dur.Value, true
	}

	if !haveKind || !haveBase {
		return 0, "", diag.Wrap("resolve", call.Pos, ErrInvalidOption, "backoff requires exponential and a base duration")
	}

	return base, kind, nil
}
