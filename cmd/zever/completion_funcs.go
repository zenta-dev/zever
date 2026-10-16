package main

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Dynamic shell completion. Every command keeps DisableFlagParsing (the
// legacy stdlib parsers stay authoritative for execution), so completion is
// wired explicitly: positional .zen files via ValidArgsFunction and flag
// values via RegisterFlagCompletionFunc on the flags documented for --help.

// completeZenFiles completes positional schema files: only .zen entries.
func completeZenFiles(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"zen"}, cobra.ShellCompDirectiveFilterFileExt
}

// completeDirs completes directories only.
func completeDirs(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs
}

// completeGenerateKinds completes the first `generate` positional with the
// generator kinds; later positionals fall back to file completion.
func completeGenerateKinds(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveDefault
	}

	return filterPrefix(allGenerate, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completeBackends completes the comma-separated --backend list, offering
// each not-yet-chosen backend after the last comma.
func completeBackends(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeCommaList(backendNamesSlice(), toComplete), cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
}

// completeProfiles completes the `new --profile` choices.
func completeProfiles(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return filterPrefix([]string{"minimal", "api", "full"}, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completeNewAdapters completes `new --adapters battery=adapter,...`: battery
// names first, then that battery's adapters after the '='.
func completeNewAdapters(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	prefix, last := splitLastComma(toComplete)

	battery, adapterPart, hasEq := strings.Cut(last, "=")
	if !hasEq {
		out := make([]string, 0, len(allServiceAdapters))

		for b := range allServiceAdapters {
			if strings.HasPrefix(b, last) {
				out = append(out, prefix+b+"=")
			}
		}

		sort.Strings(out)

		return out, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
	}

	var out []string

	for _, a := range adaptersForBattery(battery) {
		if strings.HasPrefix(a, adapterPart) {
			out = append(out, prefix+battery+"="+a)
		}
	}

	return out, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
}

// completeBuildAdapters completes `build --adapters a,b/c`: a bare battery
// name or a battery/adapter pin.
func completeBuildAdapters(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	prefix, last := splitLastComma(toComplete)

	battery, adapterPart, hasSlash := strings.Cut(last, "/")
	if !hasSlash {
		out := make([]string, 0, len(allServiceAdapters))

		for b := range allServiceAdapters {
			if strings.HasPrefix(b, last) {
				out = append(out, prefix+b)
			}
		}

		sort.Strings(out)

		return out, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
	}

	var out []string

	for _, a := range adaptersForBattery(battery) {
		if strings.HasPrefix(a, adapterPart) {
			out = append(out, prefix+battery+"/"+a)
		}
	}

	return out, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
}

// completeConfigFiles completes --config with YAML/JSON files.
func completeConfigFiles(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"yaml", "yml", "json"}, cobra.ShellCompDirectiveFilterFileExt
}

func filterPrefix(all []string, prefix string) []string {
	out := make([]string, 0, len(all))

	for _, s := range all {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}

	return out
}

// splitLastComma splits "a,b,c" into ("a,b,", "c").
func splitLastComma(s string) (prefix, last string) {
	i := strings.LastIndexByte(s, ',')

	return s[:i+1], s[i+1:]
}

// completeCommaList completes the item after the last comma, skipping items
// already present earlier in the list.
func completeCommaList(all []string, toComplete string) []string {
	prefix, last := splitLastComma(toComplete)

	chosen := map[string]bool{}

	for _, c := range strings.Split(strings.TrimSuffix(prefix, ","), ",") {
		chosen[c] = true
	}

	out := make([]string, 0, len(all))

	for _, s := range all {
		if !chosen[s] && strings.HasPrefix(s, last) {
			out = append(out, prefix+s)
		}
	}

	return out
}

// completeNothing suppresses file completion for positionals that are free
// text (e.g. the new project name).
func completeNothing(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// withFlagValues builds a ValidArgsFunction that also completes flag values.
// Cobra skips flag-value completion for commands with DisableFlagParsing
// (the legacy parsers own execution), so it is done here: when the word
// being completed is `--flag=` or follows a bare `--flag`, the registered
// func for that flag runs; otherwise positional falls through to fallback.
// Flag names themselves are still completed by Cobra from the flags
// documented on the command.
func withFlagValues(fallback cobra.CompletionFunc, flags map[string]cobra.CompletionFunc) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if rest, ok := strings.CutPrefix(toComplete, "--"); ok {
			if name, val, hasEq := strings.Cut(rest, "="); hasEq {
				if fn, known := flags[name]; known {
					return fn(cmd, args, val)
				}
			}
		}

		if n := len(args); n > 0 {
			if name, ok := strings.CutPrefix(args[n-1], "--"); ok && !strings.Contains(name, "=") {
				if fn, known := flags[name]; known {
					return fn(cmd, args, toComplete)
				}
			}
		}

		return fallback(cmd, args, toComplete)
	}
}

// registerCompletions wires the completion funcs onto the assembled tree.
// It is called from newRootCmd after every command is attached.
func registerCompletions(root *cobra.Command) {
	set := func(fn cobra.CompletionFunc, path ...string) {
		if c, _, err := root.Find(path); err == nil && c != root {
			c.ValidArgsFunction = fn
		}
	}

	for _, name := range []string{"check", "breaking", "fmt", "routes", "explain", "check-boundaries", "graph"} {
		set(completeZenFiles, name)
	}

	set(withFlagValues(completeZenFiles, map[string]cobra.CompletionFunc{
		"backend": completeBackends,
		"out":     completeDirs,
	}), "compile")
	set(completeGenerateKinds, "generate")
	set(withFlagValues(completeNothing, map[string]cobra.CompletionFunc{
		"profile":  completeProfiles,
		"adapters": completeNewAdapters,
		"dir":      completeDirs,
	}), "new")
	set(withFlagValues(completeNothing, map[string]cobra.CompletionFunc{
		"adapters": completeBuildAdapters,
	}), "build")
	set(withFlagValues(completeNothing, map[string]cobra.CompletionFunc{
		"config": completeConfigFiles,
	}), "doctor")
	set(withFlagValues(completeNothing, map[string]cobra.CompletionFunc{
		"config": completeConfigFiles,
	}), "config", "show")
}
