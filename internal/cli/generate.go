package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jmreicha/cfgctl/internal/core"
	"github.com/jmreicha/cfgctl/internal/ui"
	"github.com/spf13/cobra"
)

// printGenerateResults outputs the results of generation to stdout.
func printGenerateResults(results map[string]*core.Result) {
	for providerName, result := range results {
		fmt.Printf("\n%s:\n", sectionStyle.Render(providerName))

		printMetadataSummary(result.Metadata)

		if len(result.FilesCreated) > 0 {
			filesLabel := "Files created:"
			if result.DryRun {
				filesLabel = "Files to create:"
			}
			fmt.Println(labelStyle.Render("  " + filesLabel))
			for _, file := range result.FilesCreated {
				fmt.Printf("    - %s\n", pathStyle.Render(file))
			}
		}

		if len(result.FilesSkipped) > 0 {
			fmt.Println(labelStyle.Render("  Files skipped:"))
			for _, file := range result.FilesSkipped {
				fmt.Printf("    - %s\n", pathStyle.Render(file))
			}
		}

		if result.BackupPath != "" {
			fmt.Printf("%s %s\n", labelStyle.Render("  Backup:"), pathStyle.Render(result.BackupPath))
		}

		if len(result.Warnings) > 0 {
			fmt.Println(labelStyle.Render("  Warnings:"))
			for _, warning := range result.Warnings {
				fmt.Printf("    - %s\n", warningStyle.Render(warning))
			}
		}
	}
	fmt.Println()
}

func printMetadataSummary(metadata map[string]interface{}) {
	if len(metadata) == 0 {
		return
	}

	if clusters, ok := metadata["discovered_clusters"]; ok {
		fmt.Printf("  %s %v\n", labelStyle.Render("Clusters discovered:"), clusters)
	}
	if regions, ok := metadata["regions"]; ok {
		if rs, ok := regions.([]string); ok {
			fmt.Printf("  %s %s\n", labelStyle.Render("Regions:"), strings.Join(rs, ", "))
		}
	}
	if authMode, ok := metadata["auth_mode"]; ok {
		fmt.Printf("  %s %v\n", labelStyle.Render("Auth:"), authMode)
	}
	if mergeFiles, ok := metadata["merge_files"]; ok {
		if files, ok := mergeFiles.([]string); ok && len(files) > 0 {
			fmt.Printf("  %s %d files\n", labelStyle.Render("Merged:"), len(files))
		}
	}
	if hostsToAdd, ok := metadata["hosts_to_add"]; ok {
		fmt.Printf("  %s %v\n", labelStyle.Render("Hosts to add:"), hostsToAdd)
	}
	if hostsToUpdate, ok := metadata["hosts_to_update"]; ok {
		fmt.Printf("  %s %v\n", labelStyle.Render("Hosts to update:"), hostsToUpdate)
	}
	if hostsAdded, ok := metadata["hosts_added"]; ok {
		fmt.Printf("  %s %v\n", labelStyle.Render("Hosts added:"), hostsAdded)
	}
	if hostsUpdated, ok := metadata["hosts_updated"]; ok {
		fmt.Printf("  %s %v\n", labelStyle.Render("Hosts updated:"), hostsUpdated)
	}
	if hostsFromHistory, ok := metadata["hosts_from_history"]; ok {
		fmt.Printf("  %s %v\n", labelStyle.Render("Hosts from history:"), hostsFromHistory)
	}
	if profiles, ok := metadata["discovered_profiles"]; ok {
		fmt.Printf("  %s %v\n", labelStyle.Render("Profiles discovered:"), profiles)
	}
	if credProfiles, ok := metadata["credential_profiles"]; ok {
		fmt.Printf("  %s %v\n", labelStyle.Render("Credential profiles:"), credProfiles)
	}
	if connections, ok := metadata["connections"]; ok {
		fmt.Printf("  %s %v\n", labelStyle.Render("Connections:"), connections)
	}
}

func newGenerateCmd() *cobra.Command {
	var force bool
	var replace bool

	cmd := &cobra.Command{
		Use:   "generate [provider...]",
		Short: "Generate configuration files",
		Long: `Generate configuration files for one or more providers.
If no providers are specified, all registered providers will be executed.

Examples:
  cfgctl generate aws
  cfgctl generate kubernetes
  cfgctl generate ssh --ssh-config-path ~/.ssh
  cfgctl generate aws ssh
  cfgctl generate all
  cfgctl generate --dry-run
  cfgctl generate --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}

			ctx := context.Background()

			// Handle "all" as an explicit provider name.
			providers := args
			if len(args) == 1 && args[0] == "all" {
				providers = nil
			}

			// Validate conflicting flag combinations.
			if kubeMergeOnly && eksRegions != "" {
				return errors.New("--kube-merge-only skips EKS discovery; --eks-regions is incompatible")
			}

			// Validate provider names against the registry.
			if err := validateProviders(providers); err != nil {
				return err
			}

			opts := &core.ExecuteOptions{
				Providers: providers,
				DryRun:    dryRun,
				Force:     force,
				Replace:   replace,
				NoBackup:  noBackup,
				Verbose:   verbose,
			}

			spin := ui.NewSpinner()
			engine.SetStatus(spin)
			engine.SetPrompter(ui.Prompter{Spinner: spin})
			spin.Start("Starting generation...")

			results, err := engine.Execute(ctx, opts)
			if err != nil {
				spin.Stop()
				return err
			}

			spin.Stop()
			printGenerateResults(results)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing files")
	cmd.Flags().BoolVar(&replace, "replace", false, "discard manually-added entries and write only cfgctl-managed content")
	cmd.Flags().StringVar(&sshConfigPath, "ssh-config-path", "", "ssh config directory (default: ~/.ssh)")
	cmd.Flags().BoolVar(&awsCredentialProcess, "aws-credential-process", false, "use credential_process for AWS profiles")
	cmd.Flags().BoolVar(&awsCredentials, "aws-credentials", false, "generate AWS credentials output")
	cmd.Flags().BoolVar(&awsDemo, "aws-demo", false, "use fake AWS discovery data")
	cmd.Flags().StringVar(&awsPrefix, "aws-prefix", "", "prefix for generated AWS profile names")
	cmd.Flags().BoolVar(&awsPrune, "aws-prune", false, "remove stale AWS profiles with marker key")
	cmd.Flags().StringVar(&awsRoleFilters, "aws-roles", "", "comma-separated AWS role names")
	cmd.Flags().StringVar(&awsSSOStartURL, "aws-sso-url", "", "AWS SSO start URL")
	cmd.Flags().StringVar(&awsSSORegion, "aws-sso-region", "", "AWS SSO region")
	cmd.Flags().StringVar(&awsTemplate, "aws-template", "", "template for AWS profile names")
	cmd.Flags().BoolVar(&kubeMerge, "kube-merge", false, "merge existing kubeconfig files")
	cmd.Flags().BoolVar(&kubeMergeOnly, "kube-merge-only", false, "merge existing kubeconfig files without AWS discovery")
	cmd.Flags().StringVar(&eksRegions, "eks-regions", "", "comma-separated AWS regions for EKS discovery")
	cmd.Flags().StringVar(&eksRoles, "eks-roles", "", "comma-separated role names for EKS profile filtering")
	cmd.Flags().BoolVar(&steampipeIgnoreErrors, "steampipe-ignore-errors", false, "add ignore_error_codes (AccessDenied, UnauthorizedOperation) to all connections")
	cmd.Flags().StringVar(&steampipeRegions, "steampipe-regions", "", "comma-separated AWS regions for steampipe connections")

	return cmd
}

// validateProviders checks that each named provider exists in the registry.
// An empty list means "all providers" and is always valid.
func validateProviders(providers []string) error {
	if len(providers) == 0 {
		return nil
	}
	registered := registry.List()
	registeredSet := make(map[string]struct{}, len(registered))
	for _, p := range registered {
		registeredSet[p] = struct{}{}
	}
	for _, p := range providers {
		if _, ok := registeredSet[p]; !ok {
			return fmt.Errorf("unknown provider %q — valid providers: %s",
				p, strings.Join(registered, ", "))
		}
	}
	return nil
}
