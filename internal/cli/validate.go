package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jmreicha/cfgctl/internal/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate provider prerequisites",
		Long:  "Check if all prerequisites are met for registered providers.",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()

			fmt.Println()

			if verbose {
				configPath := viper.ConfigFileUsed()
				if configPath == "" {
					searchPaths := core.FindConfigSearchPaths()
					fmt.Printf("%s %s\n\n", labelStyle.Render("Config:"), warningStyle.Render("not found (searched: "+strings.Join(searchPaths, ", ")+")"))
				} else {
					fmt.Printf("%s %s\n\n", labelStyle.Render("Config:"), pathStyle.Render(configPath))
				}
			}

			providers := registry.GetAll()
			failed := false
			for _, provider := range providers {
				if err := provider.Validate(ctx); err != nil {
					fmt.Printf("  %s %s: %s\n", errorStyle.Render("✗"), sectionStyle.Render(provider.Name()), err)
					failed = true
				} else {
					fmt.Printf("  %s %s\n", successStyle.Render("✓"), sectionStyle.Render(provider.Name()))
				}
			}

			fmt.Println()

			if failed {
				return errors.New("validation failed")
			}

			fmt.Println(labelStyle.Render("All providers validated successfully"))
			return nil
		},
	}

	return cmd
}
