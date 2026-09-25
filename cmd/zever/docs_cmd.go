package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// newDocsCmd builds the `docs` command, which generates man pages plus
// markdown reference output into a directory.
//
// root is the tree documented (passed explicitly so the package-level rootCmd
// initializer stays cycle-free).
func newDocsCmd(root *cobra.Command) *cobra.Command {
	var dir string

	docs := &cobra.Command{
		Use:   "docs",
		Short: "Generate man pages and markdown reference docs",
		Long: `Generate man pages and markdown reference docs into a directory.

Writes one man page per command plus one markdown file per command
into the given directory.`,
		Example: `  zever docs --dir ./man`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if dir == "" {
				return errors.New("zever docs: --dir is required")
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("zever docs: create dir: %w", err)
			}

			headerDate := time.Now()
			header := &doc.GenManHeader{
				Title:   "ZEVER",
				Section: "1",
				Date:    &headerDate,
				Source:  "zever v" + cliVersion,
				Manual:  "zever manual",
			}
			if err := doc.GenManTree(root, header, dir); err != nil {
				return fmt.Errorf("zever docs: man pages: %w", err)
			}
			if err := doc.GenMarkdownTree(root, dir); err != nil {
				return fmt.Errorf("zever docs: markdown: %w", err)
			}

			return nil
		},
	}

	docs.Flags().StringVar(&dir, "dir", "", "output directory for generated docs (required)")

	return docs
}
