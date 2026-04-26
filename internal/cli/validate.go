package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate provider prerequisites",
		Long:  "Check if all prerequisites are met for registered providers.",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()

			if err := engine.ValidateAll(ctx); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			fmt.Println(labelStyle.Render("All providers validated successfully"))
			return nil
		},
	}

	return cmd
}
