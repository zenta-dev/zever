package config

import (
	"reflect"
	"sort"
	"strings"
)

// UnresolvedKey describes one environment key that sits under a known
// service prefix but resolves to no field of that service.
type UnresolvedKey struct {
	// Key is the environment variable name, for example "DB_URL".
	Key string
	// Service is the service the key sits under, for example "db".
	Service string
	// Field is the remainder after the service prefix, for example "URL".
	Field string
	// Suggestion is the closest known field of the service when one is
	// close enough to suggest, empty otherwise. A non-empty Suggestion
	// marks a near-miss typo.
	Suggestion string
}

// UnresolvedKeys lists the environment keys under a known service prefix
// that resolve to no field, sorted by key. Resolve ignores those keys
// (they are ambient variables, not typos), so this is how a caller reports
// them: a key with a non-empty Suggestion is a near-miss typo.
//
// Only the default instance of every service is inspected; named instances
// (SERVICE__NAME__FIELD) are not listed.
func UnresolvedKeys(env Env) []UnresolvedKey {
	if len(env) == 0 {
		return nil
	}
	cfg := Default()
	keys := sortedEnvKeys(env)
	var out []UnresolvedKey
	for _, svc := range knownServiceNames() {
		opts, ok := serviceRefs(cfg, svc)
		if !ok {
			continue
		}
		target := reflect.Indirect(reflect.ValueOf(opts))
		candidates := fieldCandidates(target)
		prefix := strings.ToUpper(svc)
		for _, key := range keys {
			rest, ok := applyPrefixKey(key, prefix, false)
			if !ok {
				continue
			}
			if normEqualFold(rest, "adapter") {
				// Adapter-selection keys are hard errors at
				// Resolve/ApplyEnv time, never ignored input, so they
				// are not "unresolved".
				continue
			}
			match := matchEnvField(target, rest)
			if match.Path != nil || match.Ambiguous {
				continue
			}
			out = append(out, UnresolvedKey{
				Key:        key,
				Service:    svc,
				Field:      rest,
				Suggestion: unknownEnvFieldError(svc, rest, match.InnerName, match.HasNesting, match.OuterStruct, candidates).Suggestion,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Service < out[j].Service
	})
	return out
}
