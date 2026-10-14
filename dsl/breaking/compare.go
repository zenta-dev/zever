// Package breaking compares two resolved .zen schemas and classifies every
// difference as breaking or non-breaking for API consumers, buf-breaking
// style. It depends only on dsl/ir and dsl/diag -- no LSP,
// no CLI, no backend -- so it can be driven from `zever breaking` or from
// any other caller with two compiled schemas to hand.
//
// Scope (the "core structural set"): a removed module/entity/message/
// service/operation/field is breaking; a field's type or optionality
// (optional -> required) or an operation's param/return type changing is
// breaking; an operation's HTTP method or path changing (including its
// HTTP transport being removed) is breaking. A field carrying a resolver-
// validated @renamed_from is reported as a rename, not a false-positive
// remove+add pair. Additions (new module/entity/message/field/operation)
// and new @validate rules are reported as non-breaking, informational
// changes. Tightened validation is explicitly out of scope for this first
// version.
//
// Extended structural set: named enums (removal breaking; value removal
// breaking, pure value addition informational), entity indexes (removal
// informational; adding a unique index breaking since writes can now fail),
// relations (removal or shape change breaking, addition informational),
// field defaults (adding a default informational; removing or changing one
// breaking), jobs (removal or param change breaking; queue/retry retune
// informational), schedules (removal breaking; cron/dispatch retune
// informational), and sagas (removal or step change breaking).
package breaking

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// Kind identifies the category of a detected change.
type Kind string

// Change kinds. The _removed/_changed kinds are always Breaking; the
// _added kinds are always non-breaking (informational).
const (
	KindModuleRemoved           Kind = "module_removed"
	KindModuleAdded             Kind = "module_added"
	KindEntityRemoved           Kind = "entity_removed"
	KindEntityAdded             Kind = "entity_added"
	KindFieldRemoved            Kind = "field_removed"
	KindFieldAdded              Kind = "field_added"
	KindFieldRenamed            Kind = "field_renamed"
	KindFieldTypeChanged        Kind = "field_type_changed"
	KindFieldOptionalityChanged Kind = "field_optionality_changed"
	KindMessageRemoved          Kind = "message_removed"
	KindMessageAdded            Kind = "message_added"
	KindServiceRemoved          Kind = "service_removed"
	KindServiceAdded            Kind = "service_added"
	KindOperationRemoved        Kind = "operation_removed"
	KindOperationAdded          Kind = "operation_added"
	KindOperationParamsChanged  Kind = "operation_params_changed"
	KindOperationReturnsChanged Kind = "operation_returns_changed"
	KindOperationHTTPChanged    Kind = "operation_http_changed"
	KindValidateAdded           Kind = "validate_added"
	// Extended structural set. Removed kinds are Breaking; added kinds
	// are informational, except where noted.
	KindEnumRemoved         Kind = "enum_removed"
	KindEnumAdded           Kind = "enum_added"
	KindEnumValuesChanged   Kind = "enum_values_changed"
	KindIndexRemoved        Kind = "index_removed"
	KindIndexAdded          Kind = "index_added"
	KindRelationRemoved     Kind = "relation_removed"
	KindRelationAdded       Kind = "relation_added"
	KindRelationChanged     Kind = "relation_changed"
	KindFieldDefaultChanged Kind = "field_default_changed"
	KindJobRemoved          Kind = "job_removed"
	KindJobAdded            Kind = "job_added"
	KindJobParamsChanged    Kind = "job_params_changed"
	KindJobConfigChanged    Kind = "job_config_changed"
	KindScheduleRemoved     Kind = "schedule_removed"
	KindScheduleAdded       Kind = "schedule_added"
	KindScheduleChanged     Kind = "schedule_changed"
	KindSagaRemoved         Kind = "saga_removed"
	KindSagaAdded           Kind = "saga_added"
	KindSagaStepsChanged    Kind = "saga_steps_changed"
)

// Change is one detected difference between an old and a new schema.
type Change struct {
	Kind     Kind
	Breaking bool
	Message  string
	Pos      diag.Position
}

// HasBreaking reports whether changes contains at least one Breaking change.
func HasBreaking(changes []Change) bool {
	for _, c := range changes {
		if c.Breaking {
			return true
		}
	}

	return false
}

// Compare walks every module in old and new (matched by Version+Name -- see
// moduleKey). A module present in both is compared entity by entity,
// message by message, service by service. A module removed entirely is
// reported as one breaking KindModuleRemoved change -- not silently
// skipped, since a whole-module deletion (every entity/service/RPC it
// declared, gone at once) is at least as breaking as any single removal
// Compare otherwise detects. A module added entirely is KindModuleAdded,
// non-breaking, mirroring every other _added kind.
func Compare(old, newSchema *ir.Schema) []Change {
	var changes []Change

	oldModules := modulesByKey(old)
	newModules := modulesByKey(newSchema)

	for key, oldMod := range oldModules {
		newMod, ok := newModules[key]
		if !ok {
			changes = append(changes, Change{
				Kind: KindModuleRemoved, Breaking: true, Pos: modulePos(oldMod),
				Message: fmt.Sprintf("module %q was removed", moduleLabel(oldMod)),
			})

			continue
		}

		changes = append(changes, compareEntities(oldMod, newMod)...)
		changes = append(changes, compareMessages(oldMod, newMod)...)
		changes = append(changes, compareServices(oldMod, newMod)...)
		changes = append(changes, compareEnums(oldMod, newMod)...)
		changes = append(changes, compareJobs(oldMod, newMod)...)
		changes = append(changes, compareSchedules(oldMod, newMod)...)
		changes = append(changes, compareSagas(oldMod, newMod)...)
	}

	for key, newMod := range newModules {
		if _, ok := oldModules[key]; !ok {
			changes = append(changes, Change{
				Kind: KindModuleAdded, Breaking: false, Pos: modulePos(newMod),
				Message: fmt.Sprintf("module %q was added", moduleLabel(newMod)),
			})
		}
	}

	return changes
}

// moduleLabel names m for a Change message: its declared name, or
// "(implicit)" for the unnamed sentinel module (see resolver's Module{Name:
// ""} convention).
func moduleLabel(m *ir.Module) string {
	if m == nil || m.Name == "" {
		return "(implicit)"
	}

	return m.Name
}

// modulePos picks a representative position for a whole-module Change: the
// first entity/message/service declaration position found, or the zero
// Position if the module declares nothing (an edge case with no source
// location to point at).
func modulePos(m *ir.Module) diag.Position {
	if m == nil {
		return diag.Position{}
	}

	if len(m.Entities) > 0 {
		return m.Entities[0].Pos
	}

	if len(m.Messages) > 0 {
		return m.Messages[0].Pos
	}

	if len(m.Services) > 0 {
		return m.Services[0].Pos
	}

	return diag.Position{}
}

func modulesByKey(schema *ir.Schema) map[string]*ir.Module {
	out := map[string]*ir.Module{}

	if schema == nil {
		return out
	}

	for _, m := range schema.Modules {
		out[m.Version+"\x00"+m.Name] = m
	}

	return out
}

func compareEntities(oldMod, newMod *ir.Module) []Change {
	var changes []Change

	oldEntities := entitiesByName(oldMod)
	newEntities := entitiesByName(newMod)

	for name, oldEnt := range oldEntities {
		newEnt, ok := newEntities[name]
		if !ok {
			changes = append(changes, Change{
				Kind: KindEntityRemoved, Breaking: true, Pos: oldEnt.Pos,
				Message: fmt.Sprintf("entity %q was removed", name),
			})

			continue
		}

		changes = append(changes, compareFields(name, oldEnt.Fields, newEnt.Fields)...)
		changes = append(changes, compareIndexes(name, oldEnt.Indexes, newEnt.Indexes)...)
		changes = append(changes, compareRelations(name, oldEnt.Relations, newEnt.Relations)...)
	}

	for name, newEnt := range newEntities {
		if _, ok := oldEntities[name]; !ok {
			changes = append(changes, Change{
				Kind: KindEntityAdded, Breaking: false, Pos: newEnt.Pos,
				Message: fmt.Sprintf("entity %q was added", name),
			})
		}
	}

	return changes
}

func compareMessages(oldMod, newMod *ir.Module) []Change {
	var changes []Change

	oldMessages := messagesByName(oldMod)
	newMessages := messagesByName(newMod)

	for name, oldMsg := range oldMessages {
		newMsg, ok := newMessages[name]
		if !ok {
			changes = append(changes, Change{
				Kind: KindMessageRemoved, Breaking: true, Pos: oldMsg.Pos,
				Message: fmt.Sprintf("message %q was removed", name),
			})

			continue
		}

		changes = append(changes, compareFields(name, oldMsg.Fields, newMsg.Fields)...)
	}

	for name, newMsg := range newMessages {
		if _, ok := oldMessages[name]; !ok {
			changes = append(changes, Change{
				Kind: KindMessageAdded, Breaking: false, Pos: newMsg.Pos,
				Message: fmt.Sprintf("message %q was added", name),
			})
		}
	}

	return changes
}

// compareFields compares two field lists belonging to the entity/message
// named owner, reporting renamed/removed/added fields, type and
// optionality changes for fields present in both (under their matched
// name), and new @validate rules. A field whose new-side counterpart
// carries a resolver-validated @renamed_from(oldName) is compared under
// that match and reported as a rename, never a false-positive
// remove-then-add pair.
func compareFields(owner string, oldFields, newFields []*ir.Field) []Change {
	var changes []Change

	oldByName := fieldsByName(oldFields)
	newByName := fieldsByName(newFields)

	// renameTargets maps an old field name to the new field declaring
	// @renamed_from(oldName), so a renamed field is matched by that intent
	// instead of by name.
	renameTargets := map[string]*ir.Field{}

	for _, f := range newFields {
		if f.RenamedFrom != nil {
			renameTargets[*f.RenamedFrom] = f
		}
	}

	for name, oldField := range oldByName {
		newField, ok := newByName[name]
		renamed := false

		if !ok {
			if target, isRename := renameTargets[name]; isRename {
				newField, ok, renamed = target, true, true
				changes = append(changes, Change{
					Kind: KindFieldRenamed, Breaking: false, Pos: newField.Pos,
					Message: fmt.Sprintf("%s.%s was renamed to %s.%s", owner, name, owner, newField.Name),
				})
			}
		}

		if !ok {
			changes = append(changes, Change{
				Kind: KindFieldRemoved, Breaking: true, Pos: oldField.Pos,
				Message: fmt.Sprintf("%s.%s was removed", owner, name),
			})

			continue
		}

		fieldLabel := name
		if renamed {
			fieldLabel = newField.Name
		}

		if !sameFieldType(oldField, newField) {
			changes = append(changes, Change{
				Kind: KindFieldTypeChanged, Breaking: true, Pos: newField.Pos,
				Message: fmt.Sprintf("%s.%s changed type", owner, fieldLabel),
			})
		}

		if oldField.Optional && !newField.Optional {
			changes = append(changes, Change{
				Kind: KindFieldOptionalityChanged, Breaking: true, Pos: newField.Pos,
				Message: fmt.Sprintf("%s.%s changed from optional to required", owner, fieldLabel),
			})
		}

		if !sameDefault(oldField.Default, newField.Default) {
			// Gaining a default is backward compatible (previously required
			// writes keep working); losing or changing one is not.
			breaking := oldField.Default != nil
			changes = append(changes, Change{
				Kind: KindFieldDefaultChanged, Breaking: breaking, Pos: newField.Pos,
				Message: fmt.Sprintf("%s.%s changed default", owner, fieldLabel),
			})
		}

		if len(newField.Validate) > len(oldField.Validate) {
			changes = append(changes, Change{
				Kind: KindValidateAdded, Breaking: false, Pos: newField.Pos,
				Message: fmt.Sprintf("%s.%s gained a new @validate rule", owner, fieldLabel),
			})
		}
	}

	for name, newField := range newByName {
		if _, ok := oldByName[name]; ok {
			continue
		}

		// Already reported as a rename target above; don't also report it
		// as a fresh addition.
		if newField.RenamedFrom != nil {
			if _, wasOldPresent := oldByName[*newField.RenamedFrom]; wasOldPresent {
				continue
			}
		}

		changes = append(changes, Change{
			Kind: KindFieldAdded, Breaking: false, Pos: newField.Pos,
			Message: fmt.Sprintf("%s.%s was added", owner, name),
		})
	}

	return changes
}

func compareServices(oldMod, newMod *ir.Module) []Change {
	var changes []Change

	oldServices := servicesByName(oldMod)
	newServices := servicesByName(newMod)

	for name, oldSvc := range oldServices {
		newSvc, ok := newServices[name]
		if !ok {
			changes = append(changes, Change{
				Kind: KindServiceRemoved, Breaking: true, Pos: oldSvc.Pos,
				Message: fmt.Sprintf("service %q was removed", name),
			})

			continue
		}

		changes = append(changes, compareOperations(name, oldSvc.Operations, newSvc.Operations)...)
	}

	for name, newSvc := range newServices {
		if _, ok := oldServices[name]; !ok {
			changes = append(changes, Change{
				Kind: KindServiceAdded, Breaking: false, Pos: newSvc.Pos,
				Message: fmt.Sprintf("service %q was added", name),
			})
		}
	}

	return changes
}

func compareOperations(svcName string, oldOps, newOps []*ir.Operation) []Change {
	var changes []Change

	oldByName := operationsByName(oldOps)
	newByName := operationsByName(newOps)

	for name, oldOp := range oldByName {
		newOp, ok := newByName[name]
		if !ok {
			changes = append(changes, Change{
				Kind: KindOperationRemoved, Breaking: true, Pos: oldOp.Pos,
				Message: fmt.Sprintf("rpc %s.%s was removed", svcName, name),
			})

			continue
		}

		if !sameParams(oldOp.Params, newOp.Params) {
			changes = append(changes, Change{
				Kind: KindOperationParamsChanged, Breaking: true, Pos: newOp.Pos,
				Message: fmt.Sprintf("rpc %s.%s's parameters changed", svcName, name),
			})
		}

		if !sameReturns(oldOp, newOp) {
			changes = append(changes, Change{
				Kind: KindOperationReturnsChanged, Breaking: true, Pos: newOp.Pos,
				Message: fmt.Sprintf("rpc %s.%s's return type changed", svcName, name),
			})
		}

		if !sameHTTP(oldOp, newOp) {
			changes = append(changes, Change{
				Kind: KindOperationHTTPChanged, Breaking: true, Pos: newOp.Pos,
				Message: fmt.Sprintf("rpc %s.%s's HTTP method/path changed", svcName, name),
			})
		}
	}

	for name, newOp := range newByName {
		if _, ok := oldByName[name]; !ok {
			changes = append(changes, Change{
				Kind: KindOperationAdded, Breaking: false, Pos: newOp.Pos,
				Message: fmt.Sprintf("rpc %s.%s was added", svcName, name),
			})
		}
	}

	return changes
}

// compareEnums compares the named enums of two matched modules. A removed
// enum is breaking; an added one is informational. A value removal is
// breaking (stored data or callers may name it); a pure value addition is
// informational. A reorder with the same set is not reported at all.
func compareEnums(oldMod, newMod *ir.Module) []Change {
	var changes []Change

	oldEnums := enumsByName(oldMod)
	newEnums := enumsByName(newMod)

	for name, oldEnum := range oldEnums {
		newEnum, ok := newEnums[name]
		if !ok {
			changes = append(changes, Change{
				Kind: KindEnumRemoved, Breaking: true, Pos: oldEnum.Pos,
				Message: fmt.Sprintf("enum %q was removed", name),
			})

			continue
		}

		if sameStringSlice(oldEnum.Values, newEnum.Values) {
			continue
		}

		// A pure reorder names the same set: string enums carry no order.
		if sameStringSet(oldEnum.Values, newEnum.Values) {
			continue
		}

		breaking := !isPureAddition(oldEnum.Values, newEnum.Values)
		changes = append(changes, Change{
			Kind: KindEnumValuesChanged, Breaking: breaking, Pos: newEnum.Pos,
			Message: fmt.Sprintf("enum %q values changed", name),
		})
	}

	for name, newEnum := range newEnums {
		if _, ok := oldEnums[name]; !ok {
			changes = append(changes, Change{
				Kind: KindEnumAdded, Breaking: false, Pos: newEnum.Pos,
				Message: fmt.Sprintf("enum %q was added", name),
			})
		}
	}

	return changes
}

// isPureAddition reports whether next holds every value of old (in any
// order) plus at least one more: no value was removed or renamed.
func isPureAddition(old, next []string) bool {
	if len(next) <= len(old) {
		return false
	}

	seen := make(map[string]bool, len(next))
	for _, v := range next {
		seen[v] = true
	}

	for _, v := range old {
		if !seen[v] {
			return false
		}
	}

	return true
}

// compareIndexes compares the database indexes of one entity, keyed by
// column set. A removed index is informational (read performance only); an
// added unique index is breaking (previously valid writes can now fail),
// a non-unique addition informational. A uniqueness flip on the same
// columns reports as a remove+add pair.
func compareIndexes(owner string, oldIndexes, newIndexes []*ir.Index) []Change {
	var changes []Change

	oldBySig := indexesByColumns(oldIndexes)
	newBySig := indexesByColumns(newIndexes)

	for sig, oldIdx := range oldBySig {
		newIdx, ok := newBySig[sig]
		if !ok {
			changes = append(changes, Change{
				Kind: KindIndexRemoved, Breaking: false, Pos: oldIdx.Pos,
				Message: fmt.Sprintf("%s index on (%s) was removed", owner, sig),
			})

			continue
		}

		if oldIdx.Unique != newIdx.Unique {
			changes = append(changes, Change{
				Kind: KindIndexRemoved, Breaking: false, Pos: oldIdx.Pos,
				Message: fmt.Sprintf("%s index on (%s) was removed", owner, sig),
			})
			changes = append(changes, Change{
				Kind: KindIndexAdded, Breaking: newIdx.Unique, Pos: newIdx.Pos,
				Message: fmt.Sprintf("%s index on (%s) was added", owner, sig),
			})
		}
	}

	for sig, newIdx := range newBySig {
		if _, ok := oldBySig[sig]; !ok {
			changes = append(changes, Change{
				Kind: KindIndexAdded, Breaking: newIdx.Unique, Pos: newIdx.Pos,
				Message: fmt.Sprintf("%s index on (%s) was added", owner, sig),
			})
		}
	}

	return changes
}

// indexColumns renders an index's column set as its compare key: column
// names in declaration order, comma-joined. Indexes have no names in IR,
// so the column set is the identity.
func indexColumns(idx *ir.Index) string {
	cols := make([]string, 0, len(idx.Columns))
	for _, c := range idx.Columns {
		if c != nil {
			cols = append(cols, c.Name)
		}
	}

	return strings.Join(cols, ",")
}

func indexesByColumns(indexes []*ir.Index) map[string]*ir.Index {
	out := map[string]*ir.Index{}
	for _, idx := range indexes {
		out[indexColumns(idx)] = idx
	}

	return out
}

// compareRelations compares one entity's relations, keyed by field name. A
// removed relation is breaking (callers may traverse it); an added one is
// informational; a shape change (kind, target, foreign key, delete rule,
// join table) on the same field is breaking.
func compareRelations(owner string, oldRelations, newRelations []*ir.Relation) []Change {
	var changes []Change

	oldByName := relationsByName(oldRelations)
	newByName := relationsByName(newRelations)

	for name, oldRel := range oldByName {
		newRel, ok := newByName[name]
		if !ok {
			changes = append(changes, Change{
				Kind: KindRelationRemoved, Breaking: true, Pos: oldRel.Pos,
				Message: fmt.Sprintf("%s relation %q was removed", owner, name),
			})

			continue
		}

		if !sameRelation(oldRel, newRel) {
			changes = append(changes, Change{
				Kind: KindRelationChanged, Breaking: true, Pos: newRel.Pos,
				Message: fmt.Sprintf("%s relation %q changed", owner, name),
			})
		}
	}

	for name, newRel := range newByName {
		if _, ok := oldByName[name]; !ok {
			changes = append(changes, Change{
				Kind: KindRelationAdded, Breaking: false, Pos: newRel.Pos,
				Message: fmt.Sprintf("%s relation %q was added", owner, name),
			})
		}
	}

	return changes
}

// sameRelation reports whether two relations on the same field are
// equivalent: same kind, same target entity, same foreign-key column, same
// delete rule and join table.
func sameRelation(a, b *ir.Relation) bool {
	if a.Kind != b.Kind || a.OnDelete != b.OnDelete || a.JoinTable != b.JoinTable {
		return false
	}

	if (a.Target == nil) != (b.Target == nil) {
		return false
	}

	if a.Target != nil && a.Target.Name != b.Target.Name {
		return false
	}

	if (a.ForeignKey == nil) != (b.ForeignKey == nil) {
		return false
	}

	if a.ForeignKey != nil && a.ForeignKey.Name != b.ForeignKey.Name {
		return false
	}

	return true
}

// sameDefault reports whether two field defaults are equivalent: both
// absent, or both present with the same kind and equal literal. A nil
// literal ("now") only equals another nil literal of the same kind.
func sameDefault(a, b *ir.DefaultValue) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	if a.Kind != b.Kind {
		return false
	}

	return reflect.DeepEqual(a.Lit, b.Lit)
}

// compareJobs compares two matched modules' background jobs. Removal and
// param-shape changes are breaking (producers construct the payload);
// queue/retune changes are informational.
func compareJobs(oldMod, newMod *ir.Module) []Change {
	var changes []Change

	oldJobs := jobsByName(oldMod)
	newJobs := jobsByName(newMod)

	for name, oldJob := range oldJobs {
		newJob, ok := newJobs[name]
		if !ok {
			changes = append(changes, Change{
				Kind: KindJobRemoved, Breaking: true, Pos: oldJob.Pos,
				Message: fmt.Sprintf("job %q was removed", name),
			})

			continue
		}

		if !sameParams(oldJob.Params, newJob.Params) {
			changes = append(changes, Change{
				Kind: KindJobParamsChanged, Breaking: true, Pos: newJob.Pos,
				Message: fmt.Sprintf("job %q parameters changed", name),
			})
		}

		if oldJob.Queue != newJob.Queue || !sameRetry(oldJob.Retry, newJob.Retry) {
			changes = append(changes, Change{
				Kind: KindJobConfigChanged, Breaking: false, Pos: newJob.Pos,
				Message: fmt.Sprintf("job %q queue/retry changed", name),
			})
		}
	}

	for name, newJob := range newJobs {
		if _, ok := oldJobs[name]; !ok {
			changes = append(changes, Change{
				Kind: KindJobAdded, Breaking: false, Pos: newJob.Pos,
				Message: fmt.Sprintf("job %q was added", name),
			})
		}
	}

	return changes
}

// sameRetry reports whether two retry policies match on attempts, backoff
// strategy, and base duration.
func sameRetry(a, b ir.RetryPolicy) bool {
	return a.MaxAttempts == b.MaxAttempts && a.Backoff == b.Backoff && a.Base == b.Base
}

// compareSchedules compares two matched modules' schedules. Only removal
// is breaking: a cron or dispatch retune changes when (or what) runs but
// not any consumer-facing wire shape, so it is informational.
func compareSchedules(oldMod, newMod *ir.Module) []Change {
	var changes []Change

	oldSchedules := schedulesByName(oldMod)
	newSchedules := schedulesByName(newMod)

	for name, oldSched := range oldSchedules {
		newSched, ok := newSchedules[name]
		if !ok {
			changes = append(changes, Change{
				Kind: KindScheduleRemoved, Breaking: true, Pos: oldSched.Pos,
				Message: fmt.Sprintf("schedule %q was removed", name),
			})

			continue
		}

		if oldSched.Cron != newSched.Cron || !sameDispatch(oldSched, newSched) {
			changes = append(changes, Change{
				Kind: KindScheduleChanged, Breaking: false, Pos: newSched.Pos,
				Message: fmt.Sprintf("schedule %q cron/dispatch changed", name),
			})
		}
	}

	for name, newSched := range newSchedules {
		if _, ok := oldSchedules[name]; !ok {
			changes = append(changes, Change{
				Kind: KindScheduleAdded, Breaking: false, Pos: newSched.Pos,
				Message: fmt.Sprintf("schedule %q was added", name),
			})
		}
	}

	return changes
}

// sameDispatch reports whether two schedules dispatch the same job with
// equal arguments.
func sameDispatch(a, b *ir.Schedule) bool {
	if (a.Dispatch == nil) != (b.Dispatch == nil) {
		return false
	}

	if a.Dispatch != nil && a.Dispatch.Name != b.Dispatch.Name {
		return false
	}

	return reflect.DeepEqual(a.DispatchArgs, b.DispatchArgs)
}

// compareSagas compares two matched modules' sagas. Removal is breaking,
// and so is any step change: step order, membership, RPC binding,
// compensation, or pivot all alter the compensating workflow.
func compareSagas(oldMod, newMod *ir.Module) []Change {
	var changes []Change

	oldSagas := sagasByName(oldMod)
	newSagas := sagasByName(newMod)

	for name, oldSaga := range oldSagas {
		newSaga, ok := newSagas[name]
		if !ok {
			changes = append(changes, Change{
				Kind: KindSagaRemoved, Breaking: true, Pos: oldSaga.Pos,
				Message: fmt.Sprintf("saga %q was removed", name),
			})

			continue
		}

		if !sameSagaSteps(oldSaga.Steps, newSaga.Steps) {
			changes = append(changes, Change{
				Kind: KindSagaStepsChanged, Breaking: true, Pos: newSaga.Pos,
				Message: fmt.Sprintf("saga %q steps changed", name),
			})
		}
	}

	for name, newSaga := range newSagas {
		if _, ok := oldSagas[name]; !ok {
			changes = append(changes, Change{
				Kind: KindSagaAdded, Breaking: false, Pos: newSaga.Pos,
				Message: fmt.Sprintf("saga %q was added", name),
			})
		}
	}

	return changes
}

// sameSagaSteps reports whether two step lists describe the same workflow:
// same length, same names in order, same forward/compensating RPC bindings,
// same pivots.
func sameSagaSteps(a, b []*ir.SagaStep) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].Name != b[i].Name || a[i].Pivot != b[i].Pivot {
			return false
		}

		if !sameServiceRPC(a[i].Execute, b[i].Execute) {
			return false
		}

		if (a[i].Compensate == nil) != (b[i].Compensate == nil) {
			return false
		}

		if a[i].Compensate != nil && !sameServiceRPC(*a[i].Compensate, *b[i].Compensate) {
			return false
		}
	}

	return true
}

// sameServiceRPC reports whether two RPC references name the same service
// operation. A nil service or operation only equals another nil: resolved
// steps always carry both, so nil means a malformed step, never a match.
func sameServiceRPC(a, b ir.ServiceRPC) bool {
	if (a.Service == nil) != (b.Service == nil) {
		return false
	}

	if a.Service != nil && a.Service.Name != b.Service.Name {
		return false
	}

	if (a.RPC == nil) != (b.RPC == nil) {
		return false
	}

	if a.RPC != nil && a.RPC.Name != b.RPC.Name {
		return false
	}

	return true
}

// --- name-keyed lookups, mirroring the pattern already used throughout dsl/resolver ---

func entitiesByName(m *ir.Module) map[string]*ir.Entity {
	out := map[string]*ir.Entity{}
	for _, e := range m.Entities {
		out[e.Name] = e
	}

	return out
}

func enumsByName(m *ir.Module) map[string]*ir.Enum {
	out := map[string]*ir.Enum{}
	for _, e := range m.Enums {
		out[e.Name] = e
	}

	return out
}

func jobsByName(m *ir.Module) map[string]*ir.Job {
	out := map[string]*ir.Job{}
	for _, j := range m.Jobs {
		out[j.Name] = j
	}

	return out
}

func schedulesByName(m *ir.Module) map[string]*ir.Schedule {
	out := map[string]*ir.Schedule{}
	for _, s := range m.Schedules {
		out[s.Name] = s
	}

	return out
}

func sagasByName(m *ir.Module) map[string]*ir.Saga {
	out := map[string]*ir.Saga{}
	for _, s := range m.Sagas {
		out[s.Name] = s
	}

	return out
}

func relationsByName(relations []*ir.Relation) map[string]*ir.Relation {
	out := map[string]*ir.Relation{}
	for _, r := range relations {
		out[r.FieldName] = r
	}

	return out
}

func messagesByName(m *ir.Module) map[string]*ir.Message {
	out := map[string]*ir.Message{}
	for _, msg := range m.Messages {
		out[msg.Name] = msg
	}

	return out
}

func servicesByName(m *ir.Module) map[string]*ir.Service {
	out := map[string]*ir.Service{}
	for _, s := range m.Services {
		out[s.Name] = s
	}

	return out
}

func operationsByName(ops []*ir.Operation) map[string]*ir.Operation {
	out := map[string]*ir.Operation{}
	for _, op := range ops {
		out[op.Name] = op
	}

	return out
}

func fieldsByName(fields []*ir.Field) map[string]*ir.Field {
	out := map[string]*ir.Field{}
	for _, f := range fields {
		out[f.Name] = f
	}

	return out
}

// --- type-equivalence checks ---

// sameFieldType reports whether two fields have the same resolved type: a
// Ref field must reference the same-named entity/message on both sides
// (Entity vs Message counts as different even if the name coincidentally
// matches), and a scalar field must have the same ScalarType and, for an
// enum, the same value set in the same order.
func sameFieldType(a, b *ir.Field) bool {
	if (a.Ref == nil) != (b.Ref == nil) {
		return false
	}

	if a.Ref != nil {
		return a.Ref.IsEntity() == b.Ref.IsEntity() && a.Ref.Name() == b.Ref.Name()
	}

	if a.Type.Scalar != b.Type.Scalar {
		return false
	}

	if a.Type.Scalar != ir.TEnum {
		return true
	}

	return sameStringSlice(a.Type.EnumValues, b.Type.EnumValues)
}

// sameParams reports whether two operations' parameter lists are
// equivalent: same count, same names in the same order, and each matching
// pair has the same resolved type (scalar or Ref, same rule as
// sameFieldType). Any difference in shape -- a param added, removed,
// reordered, renamed, or retyped -- is treated as a breaking signature
// change, since callers construct requests positionally/by name today.
func sameParams(a, b []*ir.Param) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}

		if (a[i].Ref == nil) != (b[i].Ref == nil) {
			return false
		}

		if a[i].Ref != nil {
			if a[i].Ref.IsEntity() != b[i].Ref.IsEntity() || a[i].Ref.Name() != b[i].Ref.Name() {
				return false
			}

			continue
		}

		if a[i].Type.Scalar != b[i].Type.Scalar {
			return false
		}
	}

	return true
}

// sameReturns reports whether two operations' response shapes are
// equivalent: same Returns type (Entity vs Message, same name) and the
// same Paginated flag (pagination changes the wire shape from a single
// item to an {items, next_cursor} envelope).
func sameReturns(a, b *ir.Operation) bool {
	if a.Paginated != b.Paginated {
		return false
	}

	if (a.Returns == nil) != (b.Returns == nil) {
		return false
	}

	if a.Returns == nil {
		return true
	}

	return a.Returns.IsEntity() == b.Returns.IsEntity() && a.Returns.Name() == b.Returns.Name()
}

// sameHTTP reports whether two operations' HTTP bindings are equivalent:
// both absent, or both present with the same method and path.
func sameHTTP(a, b *ir.Operation) bool {
	ah, aok := findHTTPTransport(a.Transports)
	bh, bok := findHTTPTransport(b.Transports)

	if aok != bok {
		return false
	}

	if !aok {
		return true
	}

	return ah.Method == bh.Method && ah.Path == bh.Path
}

func findHTTPTransport(transports []ir.Transport) (ir.HTTPTransport, bool) {
	for _, t := range transports {
		if h, ok := t.(ir.HTTPTransport); ok {
			return h, true
		}
	}

	return ir.HTTPTransport{}, false
}

// sameStringSet reports whether a and b name the same values regardless
// of order.
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	seen := make(map[string]int, len(a))
	for _, v := range a {
		seen[v]++
	}

	for _, v := range b {
		seen[v]--
		if seen[v] < 0 {
			return false
		}
	}

	return true
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
