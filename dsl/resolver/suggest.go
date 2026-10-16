package resolver

import (
	"sort"

	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// hint is diag.HintSuffix scoped to this package: "" when nothing is close,
// else ` (did you mean "x"?)`, ready to append to a diagnostic message.
func hint(input string, candidates []string) string {
	return diag.HintSuffix(input, candidates)
}

// scalarCandidates returns every resolvable field-type name: the builtin
// scalars plus every declared named enum.
func scalarCandidates(enumByName map[string]*ir.Enum) []string {
	out := ScalarTypeNames()
	for name := range enumByName {
		out = append(out, name)
	}

	sort.Strings(out)

	return out
}

// namedTypeCandidates returns every entity and message name, the vocabulary
// for rpc-returns and ref positions.
func namedTypeCandidates(entityByName map[string]*ir.Entity, messageByName map[string]*ir.Message) []string {
	out := make([]string, 0, len(entityByName)+len(messageByName))
	for name := range entityByName {
		out = append(out, name)
	}

	for name := range messageByName {
		out = append(out, name)
	}

	sort.Strings(out)

	return out
}

// entityCandidates returns every declared entity name.
func entityCandidates(entityByName map[string]*ir.Entity) []string {
	out := make([]string, 0, len(entityByName))
	for name := range entityByName {
		out = append(out, name)
	}

	sort.Strings(out)

	return out
}

// errorCodeCandidates returns the gRPC-canonical error-code vocabulary.
func errorCodeCandidates() []string {
	out := make([]string, 0, len(ValidErrorCodes))
	for name := range ValidErrorCodes {
		out = append(out, name)
	}

	sort.Strings(out)

	return out
}

// fieldCandidates returns every declared field name on ent.
func fieldCandidates(ent *ir.Entity) []string {
	out := make([]string, 0, len(ent.Fields))
	for _, f := range ent.Fields {
		out = append(out, f.Name)
	}

	sort.Strings(out)

	return out
}

// serviceCandidates returns every resolved service name.
func serviceCandidates(serviceByName map[string]*ir.Service) []string {
	out := make([]string, 0, len(serviceByName))
	for name := range serviceByName {
		out = append(out, name)
	}

	sort.Strings(out)

	return out
}
