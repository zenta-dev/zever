package main

import (
	"github.com/spf13/cobra"
)

// newCompletionCmd builds the `completion` parent command with one
// subcommand per shell. Each subcommand writes that shell's completion
// script to stdout; install output per the printed Long text.
//
// root is the tree the generated scripts complete (passed explicitly so the
// package-level rootCmd initializer stays cycle-free).
func newCompletionCmd(root *cobra.Command) *cobra.Command {
	completion := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts.

Pipe the output into your shell's completion directory or source it
from your shell profile.`,
		Example: `  zever completion bash > /etc/bash_completion.d/zever
  zever completion zsh > "${fpath[1]}/_zever"
  zever completion fish > ~/.config/fish/completions/zever.fish
  zever completion powershell | Out-String | Invoke-Expression`,
		Args: cobra.NoArgs,
	}

	bash := &cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion script",
		Long: `Generate the bash completion script (v2, with descriptions).

To load completions in your current shell session:

  source <(zever completion bash)

To load completions for every new session, run once:

  zever completion bash > /etc/bash_completion.d/zever`,
		Example: `  zever completion bash > /etc/bash_completion.d/zever`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
		},
	}

	zsh := &cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion script",
		Long: `Generate the zsh completion script.

To load completions in your current shell session:

  source <(zever completion zsh)

To load completions for every new session, run once:

  zever completion zsh > "${fpath[1]}/_zever"`,
		Example: `  zever completion zsh > "${fpath[1]}/_zever"`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenZshCompletion(cmd.OutOrStdout())
		},
	}

	fish := &cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		Long: `Generate the fish completion script (with descriptions).

To load completions in your current shell session:

  zever completion fish | source

To load completions for every new session, run once:

  zever completion fish > ~/.config/fish/completions/zever.fish`,
		Example: `  zever completion fish > ~/.config/fish/completions/zever.fish`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		},
	}

	powershell := &cobra.Command{
		Use:   "powershell",
		Short: "Generate PowerShell completion script",
		Long: `Generate the PowerShell completion script (with descriptions).

To load completions in your current shell session:

  zever completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the above line to
your PowerShell profile.`,
		Example: `  zever completion powershell | Out-String | Invoke-Expression`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		},
	}

	completion.AddCommand(bash, zsh, fish, powershell)

	return completion
}
