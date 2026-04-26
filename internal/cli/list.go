package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available providers",
		Long:  "List all registered configuration providers.",
		RunE: func(_ *cobra.Command, _ []string) error {
			providers := registry.List()

			if len(providers) == 0 {
				fmt.Println("No providers registered")
				return nil
			}

			fmt.Println(labelStyle.Render("Available providers:"))
			for _, name := range providers {
				fmt.Printf("  - %s\n", sectionStyle.Render(name))
			}

			return nil
		},
	}

	return cmd
}
