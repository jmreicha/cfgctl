package cli

import (
	"fmt"

	"github.com/jmreicha/cfgctl/internal/core"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available providers",
		Long:  "List all registered configuration providers.",
		RunE: func(_ *cobra.Command, _ []string) error {
			providers := registry.GetAll()

			fmt.Println()

			if len(providers) == 0 {
				fmt.Println("  No providers registered")
				return nil
			}

			for _, p := range providers {
				if verbose {
					path := ""
					if cp, ok := p.(core.ConfigPather); ok {
						path = cp.ConfigPath()
					}
					fmt.Printf("  %s %s\n", sectionStyle.Render(p.Name()), pathStyle.Render("("+path+")"))
				} else {
					fmt.Printf("  %s\n", sectionStyle.Render(p.Name()))
				}
			}

			fmt.Println()
			return nil
		},
	}

	return cmd
}
