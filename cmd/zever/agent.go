package main

import (
	"encoding/json"
	"io"
	"sort"

	"github.com/spf13/pflag"
)

// agent.go implements --help --agent: a machine-readable command catalog for
// coding agents. It derives command names and descriptions from the Cobra
// root at runtime (no static table to drift) and documents the global flags
// and exit-code contract. It adds no subcommand, so dispatch tables are
// untouched.

// agentHelpRequested reports whether args combine a help request with
// --agent, e.g. `zever --help --agent`.
func agentHelpRequested(args []string) bool {
	var help, agent bool

	for _, a := range args {
		switch a {
		case "-h", "--help", "help":
			help = true
		case "--agent":
			agent = true
		}
	}

	return help && agent
}

// commandSpec describes one subcommand for machine consumption.
type commandSpec struct {
	Name  string `json:"name"`
	Short string `json:"short"`
}

// flagSpec describes one flag for machine consumption.
type flagSpec struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default"`
	Usage     string `json:"usage"`
}

// exitCodeSpec documents one exit code.
type exitCodeSpec struct {
	Code    int    `json:"code"`
	Meaning string `json:"meaning"`
}

// agentCatalog is the machine-readable CLI descriptor.
type agentCatalog struct {
	Commands    []commandSpec  `json:"commands"`
	GlobalFlags []flagSpec     `json:"globalFlags"`
	ExitCodes   []exitCodeSpec `json:"exitCodes"`
	JSONMode    string         `json:"jsonMode"`
}

// printAgentHelp writes the command catalog as JSON to w.
func printAgentHelp(w io.Writer) {
	catalog := agentCatalog{
		Commands:    agentCommands(),
		GlobalFlags: agentGlobalFlags(),
		ExitCodes: []exitCodeSpec{
			{Code: ExitOK, Meaning: "success"},
			{Code: ExitError, Meaning: "runtime error"},
			{Code: ExitUsage, Meaning: "flag misuse"},
		},
		JSONMode: "pass --json (or ZEVER_JSON=1) for a JSON envelope on stdout; " +
			"compile, check, explain and doctor emit structured data, other " +
			"commands emit JSON error envelopes",
	}

	_ = json.NewEncoder(w).Encode(catalog)
}

// agentCommands lists every Cobra subcommand sorted by name.
func agentCommands() []commandSpec {
	cmds := rootCmd.Commands()

	out := make([]commandSpec, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, commandSpec{Name: c.Name(), Short: c.Short})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// agentGlobalFlags lists the persistent global flags sorted by name.
func agentGlobalFlags() []flagSpec {
	var out []flagSpec

	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		out = append(out, flagSpec{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Type:      f.Value.Type(),
			Default:   f.DefValue,
			Usage:     f.Usage,
		})
	})

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}
