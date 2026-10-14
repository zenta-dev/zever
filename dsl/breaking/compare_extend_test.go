package breaking

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func TestCompareEnumRemoved(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "enum Status { active, inactive }\n\nentity Task {\n\tid: uuid @primary\n\tstatus: Status\n}")
	next := resolveSrc(t, "a.zen", "entity Task {\n\tid: uuid @primary\n\tstatus: string\n}")

	assertHasBreaking(t, Compare(old, next), KindEnumRemoved)
}

func TestCompareEnumAdded(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity Task {\n\tid: uuid @primary\n}")
	next := resolveSrc(t, "a.zen", "enum Status { active, inactive }\n\nentity Task {\n\tid: uuid @primary\n}")

	assertHasNonBreaking(t, Compare(old, next), KindEnumAdded)
}

func TestCompareEnumValueRemovedIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "enum Status { active, inactive, banned }")
	next := resolveSrc(t, "a.zen", "enum Status { active, inactive }")

	assertHasBreaking(t, Compare(old, next), KindEnumValuesChanged)
}

func TestCompareEnumValueAddedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "enum Status { active, inactive }")
	next := resolveSrc(t, "a.zen", "enum Status { active, inactive, banned }")

	assertHasNonBreaking(t, Compare(old, next), KindEnumValuesChanged)
}

func TestCompareEnumReorderIsSilent(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "enum Status { active, inactive }")
	next := resolveSrc(t, "a.zen", "enum Status { inactive, active }")

	for _, c := range Compare(old, next) {
		if c.Kind == KindEnumValuesChanged {
			t.Fatalf("pure reorder reported as a change: %+v", c)
		}
	}
}

func TestCompareIndexUniqueFlipReportsPair(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\temail: string\n\n\tindex(email)\n}")
	next := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\temail: string\n\n\tindex(email) @unique\n}")

	changes := Compare(old, next)
	assertHasNonBreaking(t, changes, KindIndexRemoved)
	assertHasBreaking(t, changes, KindIndexAdded)
}

func TestCompareIndexAddedUniqueIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\temail: string\n}")
	next := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\temail: string\n\n\tindex(email) @unique\n}")

	assertHasBreaking(t, Compare(old, next), KindIndexAdded)
}

func TestCompareIndexAddedNonUniqueIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\temail: string\n}")
	next := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\temail: string\n\n\tindex(email)\n}")

	assertHasNonBreaking(t, Compare(old, next), KindIndexAdded)
}

func TestCompareIndexRemovedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\temail: string\n\n\tindex(email)\n}")
	next := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\temail: string\n}")

	assertHasNonBreaking(t, Compare(old, next), KindIndexRemoved)
}

func TestCompareRelationRemovedIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\n\thas_many orders: Order @foreign_key(user_id)\n}\n\nentity Order {\n\tid: uuid @primary\n\tuser_id: uuid\n}")
	next := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n}\n\nentity Order {\n\tid: uuid @primary\n\tuser_id: uuid\n}")

	assertHasBreaking(t, Compare(old, next), KindRelationRemoved)
}

func TestCompareRelationAddedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n}\n\nentity Order {\n\tid: uuid @primary\n\tuser_id: uuid\n}")
	next := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\n\thas_many orders: Order @foreign_key(user_id)\n}\n\nentity Order {\n\tid: uuid @primary\n\tuser_id: uuid\n}")

	assertHasNonBreaking(t, Compare(old, next), KindRelationAdded)
}

func TestCompareRelationChangedIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity Order {\n\tid: uuid @primary\n\tuser_id: uuid\n\n\tbelongs_to user: User @foreign_key(user_id) @on_delete(cascade)\n}\n\nentity User {\n\tid: uuid @primary\n}")
	next := resolveSrc(t, "a.zen", "entity Order {\n\tid: uuid @primary\n\tuser_id: uuid\n\n\tbelongs_to user: User @foreign_key(user_id) @on_delete(set_null)\n}\n\nentity User {\n\tid: uuid @primary\n}")

	assertHasBreaking(t, Compare(old, next), KindRelationChanged)
}

func TestCompareDefaultAddedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\tname: string\n}")
	next := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\tname: string @default(\"anon\")\n}")

	assertHasNonBreaking(t, Compare(old, next), KindFieldDefaultChanged)
}

func TestCompareDefaultRemovedIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\tname: string @default(\"anon\")\n}")
	next := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\tname: string\n}")

	assertHasBreaking(t, Compare(old, next), KindFieldDefaultChanged)
}

func TestCompareDefaultValueChangedIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\tname: string @default(\"anon\")\n}")
	next := resolveSrc(t, "a.zen", "entity User {\n\tid: uuid @primary\n\tname: string @default(\"unknown\")\n}")

	assertHasBreaking(t, Compare(old, next), KindFieldDefaultChanged)
}

func TestCompareJobRemovedIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "job SendMail(user_id: uuid) {\n\tqueue: default\n\tretry: max_attempts(3), backoff(exponential, base: 30s)\n}")
	next := resolveSrc(t, "a.zen", "")

	assertHasBreaking(t, Compare(old, next), KindJobRemoved)
}

func TestCompareJobAddedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "")
	next := resolveSrc(t, "a.zen", "job SendMail(user_id: uuid) {\n\tqueue: default\n\tretry: max_attempts(3), backoff(exponential, base: 30s)\n}")

	assertHasNonBreaking(t, Compare(old, next), KindJobAdded)
}

func TestCompareJobParamsChangedIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "job SendMail(user_id: uuid) {\n\tqueue: default\n\tretry: max_attempts(3), backoff(exponential, base: 30s)\n}")
	next := resolveSrc(t, "a.zen", "job SendMail(user_id: uuid, subject: string) {\n\tqueue: default\n\tretry: max_attempts(3), backoff(exponential, base: 30s)\n}")

	assertHasBreaking(t, Compare(old, next), KindJobParamsChanged)
}

func TestCompareJobConfigChangedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "job SendMail(user_id: uuid) {\n\tqueue: default\n\tretry: max_attempts(3), backoff(exponential, base: 30s)\n}")
	next := resolveSrc(t, "a.zen", "job SendMail(user_id: uuid) {\n\tqueue: priority\n\tretry: max_attempts(3), backoff(exponential, base: 30s)\n}")

	assertHasNonBreaking(t, Compare(old, next), KindJobConfigChanged)
}

func TestCompareScheduleRemovedIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "job Cleanup() {\n\tqueue: default\n\tretry: max_attempts(2), backoff(exponential, base: 30s)\n}\n\nschedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: Cleanup()\n}")
	next := resolveSrc(t, "a.zen", "job Cleanup() {\n\tqueue: default\n\tretry: max_attempts(2), backoff(exponential, base: 30s)\n}")

	assertHasBreaking(t, Compare(old, next), KindScheduleRemoved)
}

func TestCompareScheduleAddedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "job Cleanup() {\n\tqueue: default\n\tretry: max_attempts(2), backoff(exponential, base: 30s)\n}")
	next := resolveSrc(t, "a.zen", "job Cleanup() {\n\tqueue: default\n\tretry: max_attempts(2), backoff(exponential, base: 30s)\n}\n\nschedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: Cleanup()\n}")

	assertHasNonBreaking(t, Compare(old, next), KindScheduleAdded)
}

func TestCompareScheduleCronChangedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", "job Cleanup() {\n\tqueue: default\n\tretry: max_attempts(2), backoff(exponential, base: 30s)\n}\n\nschedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: Cleanup()\n}")
	next := resolveSrc(t, "a.zen", "job Cleanup() {\n\tqueue: default\n\tretry: max_attempts(2), backoff(exponential, base: 30s)\n}\n\nschedule Nightly {\n\tcron: \"0 6 * * *\"\n\tdispatch: Cleanup()\n}")

	assertHasNonBreaking(t, Compare(old, next), KindScheduleChanged)
}

const sagaServicesFixture = `entity Item {
	id: uuid @primary
}

service StockService {
	rpc Reserve(id: uuid) -> Item {
		http: GET "/v1/stock/{id}"
		auth: required
	}

	rpc Release(id: uuid) -> Item {
		http: GET "/v1/release/{id}"
		auth: required
	}
}

service PaymentService {
	rpc Charge(id: uuid) -> Item {
		http: GET "/v1/charge/{id}"
		auth: required
	}

	rpc Refund(id: uuid) -> Item {
		http: GET "/v1/refund/{id}"
		auth: required
	}
}
`

func TestCompareSagaRemovedIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", sagaServicesFixture+`
saga Checkout {
	step reserve {
		execute: StockService.Reserve
		compensate: StockService.Release
	}
	step charge {
		execute: PaymentService.Charge
		pivot: true
	}
}
`)
	next := resolveSrc(t, "a.zen", sagaServicesFixture)

	assertHasBreaking(t, Compare(old, next), KindSagaRemoved)
}

func TestCompareSagaAddedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", sagaServicesFixture)
	next := resolveSrc(t, "a.zen", sagaServicesFixture+`
saga Checkout {
	step reserve {
		execute: StockService.Reserve
		compensate: StockService.Release
	}
	step charge {
		execute: PaymentService.Charge
		pivot: true
	}
}
`)

	assertHasNonBreaking(t, Compare(old, next), KindSagaAdded)
}

func TestCompareSagaStepsChangedIsBreaking(t *testing.T) {
	t.Parallel()

	old := resolveSrc(t, "a.zen", sagaServicesFixture+`
saga Checkout {
	step reserve {
		execute: StockService.Reserve
		compensate: StockService.Release
	}
	step charge {
		execute: PaymentService.Charge
		pivot: true
	}
}
`)
	next := resolveSrc(t, "a.zen", sagaServicesFixture+`
saga Checkout {
	step reserve {
		execute: StockService.Reserve
		compensate: StockService.Release
	}
	step charge {
		execute: PaymentService.Charge
		compensate: PaymentService.Refund
		pivot: true
	}
}
`)

	assertHasBreaking(t, Compare(old, next), KindSagaStepsChanged)
}

// TestSameRelationNilTarget pins the nil-guard branches: a dangling
// relation never equals a resolved one.
func TestSameRelationNilTarget(t *testing.T) {
	t.Parallel()

	target := &ir.Entity{Name: "User"}
	withTarget := &ir.Relation{Kind: ir.BelongsTo, FieldName: "user", Target: target}
	withoutTarget := &ir.Relation{Kind: ir.BelongsTo, FieldName: "user"}

	if sameRelation(withTarget, withoutTarget) {
		t.Fatal("sameRelation(target, nil) = true, want false")
	}

	if sameRelation(withoutTarget, withTarget) {
		t.Fatal("sameRelation(nil, target) = true, want false")
	}

	renamed := &ir.Relation{Kind: ir.BelongsTo, FieldName: "user", Target: &ir.Entity{Name: "Admin"}}
	if sameRelation(withTarget, renamed) {
		t.Fatal("sameRelation across targets = true, want false")
	}

	otherKind := &ir.Relation{Kind: ir.HasMany, FieldName: "user", Target: target}
	if sameRelation(withTarget, otherKind) {
		t.Fatal("sameRelation across kinds = true, want false")
	}
}

// TestSameDispatchNil pins the dispatch-nil branches.
func TestSameDispatchNil(t *testing.T) {
	t.Parallel()

	job := &ir.Job{Name: "J"}
	withJob := &ir.Schedule{Name: "S", Cron: "0 0 * * *", Dispatch: job}
	withoutJob := &ir.Schedule{Name: "S", Cron: "0 0 * * *"}

	if sameDispatch(withJob, withoutJob) {
		t.Fatal("sameDispatch(job, nil) = true, want false")
	}

	retargeted := &ir.Schedule{Name: "S", Cron: "0 0 * * *", Dispatch: &ir.Job{Name: "K"}}
	if sameDispatch(withJob, retargeted) {
		t.Fatal("sameDispatch across jobs = true, want false")
	}
}

// TestSameServiceRPCNil pins the malformed-step guards.
func TestSameServiceRPCNil(t *testing.T) {
	t.Parallel()

	svc := &ir.Service{Name: "S"}
	op := &ir.Operation{Name: "Op"}
	full := ir.ServiceRPC{Service: svc, RPC: op}

	if sameServiceRPC(full, ir.ServiceRPC{RPC: op}) {
		t.Fatal("sameServiceRPC with nil service = true, want false")
	}

	if sameServiceRPC(full, ir.ServiceRPC{Service: svc}) {
		t.Fatal("sameServiceRPC with nil rpc = true, want false")
	}
}

// TestSameDefaultKinds pins default comparison across kinds.
func TestSameDefaultKinds(t *testing.T) {
	t.Parallel()

	if !sameDefault(nil, nil) {
		t.Fatal("sameDefault(nil, nil) = false, want true")
	}

	if sameDefault(nil, &ir.DefaultValue{Kind: ir.DefaultNow}) {
		t.Fatal("sameDefault(nil, now) = true, want false")
	}

	if sameDefault(&ir.DefaultValue{Kind: ir.DefaultNow}, &ir.DefaultValue{Kind: ir.DefaultLiteral, Lit: "x"}) {
		t.Fatal("sameDefault across kinds = true, want false")
	}
}

// TestCompareExtendedIdenticalIsSilent guards the new comparators against
// false positives: a schema using every extended feature compares clean
// against itself.
func TestCompareExtendedIdenticalIsSilent(t *testing.T) {
	t.Parallel()

	src := `enum Status { active, inactive }

entity User {
	id: uuid @primary
	name: string @default("anon")
	status: Status

	has_many orders: Order @foreign_key(user_id)

	index(name)
	index(status) @unique
}

entity Order {
	id: uuid @primary
	user_id: uuid
}

job SendMail(user_id: uuid) {
	queue: default
	retry: max_attempts(3), backoff(exponential, base: 30s)
}

job Sweep() {
	queue: default
	retry: max_attempts(2), backoff(exponential, base: 30s)
}

schedule Nightly {
	cron: "0 0 * * *"
	dispatch: Sweep()
}

` + sagaServicesFixture + `
saga Checkout {
	step reserve {
		execute: StockService.Reserve
		compensate: StockService.Release
	}
	step charge {
		execute: PaymentService.Charge
		pivot: true
	}
}
`
	old := resolveSrc(t, "a.zen", src)
	next := resolveSrc(t, "a.zen", src)

	if changes := Compare(old, next); len(changes) != 0 {
		t.Fatalf("identical extended schemas should produce zero changes, got %+v", changes)
	}
}
