package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate documentation for gctx",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		docDir := filepath.Join(cwd, "readthedocs", "docs")
		
		if err := os.MkdirAll(docDir, 0755); err != nil {
			return err
		}

		fmt.Printf("Generating documentation in %s...\n", docDir)

		// Create a custom header function if needed, or just use default
		err := doc.GenMarkdownTree(rootCmd, docDir)
		if err != nil {
			return err
		}

		fmt.Println("Documentation generated successfully!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(docsCmd)
}
