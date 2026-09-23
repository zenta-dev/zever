// Package breaking compares two resolved .zen schemas and classifies every
// difference as breaking or non-breaking for API consumers, buf-breaking
// style. It depends only on internal/dsl/ir and internal/dsl/diag -- no LSP,
// no CLI, no backend -- so it can be driven from `zever breaking` or from
// any other caller with two compiled schemas to hand.
//
// Scope (the "core structural set"): a removed entity/message/service/
// operation/field is breaking; a field's type or an operation's param/
// return type changing is breaking; an operation's HTTP method or path
// changing (including its HTTP transport being removed) is breaking.
// Additions (new entity/message/field/operation) and new @validate rules
// are reported as non-breaking, informational changes. Comparing module
// removal/addition, and tightened validation, are explicitly out of scope
// for this first version.
package breaking

import (
	"fmt"

	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// Kind identifies the category of a detected change.
type Kind string

// Change kinds. The _removed/_changed kinds are always Breaking; the
// _added kinds are always non-breaking (informational).
const (
	KindEntityRemoved           Kind = "entity_removed"
	KindEntityAdded             Kind = "entity_added"
	KindFieldRemoved            Kind = "field_removed"
	KindFieldAdded              Kind = "field_added"
	KindFieldTypeChanged        Kind = "field_type_changed"
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

// Compare walks every module common to both old and new (matched by
// Version+Name -- see moduleKey) and reports every entity/message/service/
// operation/field difference within it. A module present in only one of the
// two schemas is not compared further -- see the package doc comment.
func Compare(old, newSchema *ir.Schema) []Change {
	var changes []Change

	oldModules := modulesByKey(old)
	newModules := modulesByKey(newSchema)

	for key, oldMod := range oldModules {
		newMod, ok := newModules[key]
		if !ok {
			continue
		}

		changes = append(changes, compareEntities(oldMod, newMod)...)
		changes = append(changes, compareMessages(oldMod, newMod)...)
		changes = append(changes, compareServices(oldMod, newMod)...)
	}

	return changes
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
// named owner, reporting removed/added fields and type changes for fields
// present in both. New @validate rules on an otherwise-unchanged field are
// reported as non-breaking.
func compareFields(owner string, oldFields, newFields []*ir.Field) []Change {
	var changes []Change

	oldByName := fieldsByName(oldFields)
	newByName := fieldsByName(newFields)

	for name, oldField := range oldByName {
		newField, ok := newByName[name]
		if !ok {
			changes = append(changes, Change{
				Kind: KindFieldRemoved, Breaking: true, Pos: oldField.Pos,
				Message: fmt.Sprintf("%s.%s was removed", owner, name),
			})

			continue
		}

		if !sameFieldType(oldField, newField) {
			changes = append(changes, Change{
				Kind: KindFieldTypeChanged, Breaking: true, Pos: newField.Pos,
				Message: fmt.Sprintf("%s.%s changed type", owner, name),
			})
		}

		if len(newField.Validate) > len(oldField.Validate) {
			changes = append(changes, Change{
				Kind: KindValidateAdded, Breaking: false, Pos: newField.Pos,
				Message: fmt.Sprintf("%s.%s gained a new @validate rule", owner, name),
			})
		}
	}

	for name, newField := range newByName {
		if _, ok := oldByName[name]; !ok {
			changes = append(changes, Change{
				Kind: KindFieldAdded, Breaking: false, Pos: newField.Pos,
				Message: fmt.Sprintf("%s.%s was added", owner, name),
			})
		}
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

// --- name-keyed lookups, mirroring the pattern already used throughout internal/dsl/resolver ---

func entitiesByName(m *ir.Module) map[string]*ir.Entity {
	out := map[string]*ir.Entity{}
	for _, e := range m.Entities {
		out[e.Name] = e
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
