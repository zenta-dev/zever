package main

import (
	"flag"
	"fmt"
	"strings"
)

// rejectUnknownFlags reports the first flag-like token in args that is not
// one of known (bare names, without dashes; "name=value" matches "name").
// Commands without their own flags pass nil and reject every flag-like
// token. The "--" separator ends the scan, and -h/--help/help always pass
// through so each command's own help handling keeps working. A close miss
// names the intended flag, mirroring the backend suggestion in
// resolveBackends.
func rejectUnknownFlags(command string, args []string, known []string) error {
	allowed := make(map[string]bool, len(known))
	prefixed := make([]string, 0, len(known))

	for _, k := range known {
		allowed[k] = true
		prefixed = append(prefixed, "--"+k)
	}

	for _, a := range args {
		if a == "--" {
			break
		}

		if a == "-h" || a == "--help" || a == "help" {
			continue
		}

		if len(a) < 2 || a[0] != '-' {
			continue
		}

		name := strings.TrimLeft(a, "-")
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}

		if allowed[name] {
			continue
		}

		if s := closest("--"+name, prefixed); s != "" {
			return fmt.Errorf("zever %s: unknown flag %q (did you mean %q?)", command, a, s)
		}

		return fmt.Errorf("zever %s: unknown flag %q", command, a)
	}

	return nil
}

// flexibleParse allows flags and positionals in any order.
//
// It splits args into flagArgs and posArgs, handling:
//   - --flag=value (already complete)
//   - --flag value (consume next token if flag needs value and next token not flag-like)
//   - bool flags (detected via FlagSet, DefValue or IsBoolFlag)
//   - short flags like -i
//   - -- terminator (rest goes to posArgs)
//
// After splitting, it calls fs.Parse(flagArgs) then returns append(posArgs, fs.Args()...).
func flexibleParse(fs *flag.FlagSet, args []string) ([]string, error) {
	var (
		flagArgs []string
		posArgs  []string
	)

	for i := 0; i < len(args); {
		arg := args[i]
		if arg == "--" {
			posArgs = append(posArgs, args[i+1:]...)
			break
		}

		if arg == "-" || !strings.HasPrefix(arg, "-") {
			posArgs = append(posArgs, arg)
			i++

			continue
		}
		// Flag-like token.
		// If it contains '=', it's already a complete flag=value.
		if strings.Contains(arg, "=") {
			flagArgs = append(flagArgs, arg)
			i++

			continue
		}
		// Extract flag name without leading dashes.
		name := strings.TrimLeft(arg, "-")

		f := fs.Lookup(name)
		if f == nil {
			// Unknown flag: let fs.Parse report the error.
			flagArgs = append(flagArgs, arg)
			i++

			continue
		}

		isBool := false
		if f.DefValue == "true" || f.DefValue == "false" {
			isBool = true
		} else if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			isBool = true
		}

		if isBool {
			flagArgs = append(flagArgs, arg)
			i++

			continue
		}
		// Value flag without '=': consume next token if it exists and is not flag-like.
		if i+1 < len(args) && args[i+1] != "--" && !strings.HasPrefix(args[i+1], "-") {
			flagArgs = append(flagArgs, arg, args[i+1])
			i += 2
		} else {
			flagArgs = append(flagArgs, arg)
			i++
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return nil, err
	}

	result := append([]string{}, posArgs...)
	result = append(result, fs.Args()...)

	return result, nil
}
