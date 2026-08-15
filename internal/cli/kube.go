package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

var kubeconfig string

func newKubeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kube",
		Short: "Manage kubeconfig files",
		Long: `Manage kubeconfig contexts, merge files, and export configurations.

These commands operate directly on kubeconfig files without requiring
cfgctl provider configuration.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			viper.SetEnvPrefix("CFGCTL")
			viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
			viper.AutomaticEnv()
			_ = viper.BindPFlag("no-backup", cmd.Root().PersistentFlags().Lookup("no-backup"))
			if !cmd.Root().PersistentFlags().Changed("no-backup") {
				noBackup = viper.GetBool("no-backup")
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig file (default: $KUBECONFIG or ~/.kube/config)")

	cmd.AddCommand(newKubeMergeCmd())
	cmd.AddCommand(newKubeDeleteCmd())
	cmd.AddCommand(newKubeRenameCmd())
	cmd.AddCommand(newKubeExportCmd())

	return cmd
}

func resolveKubeconfig() string {
	if kubeconfig != "" {
		return kubeconfig
	}
	if env := os.Getenv("KUBECONFIG"); env != "" {
		if paths := filepath.SplitList(env); len(paths) > 0 {
			return paths[0]
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}

func loadKubeconfig(path string) (*api.Config, error) {
	config, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %s: %w", path, err)
	}
	return config, nil
}

func writeKubeconfig(path string, config *api.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return clientcmd.WriteToFile(*config, path)
}

func backupKubeconfig(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is resolved from flag/env/default, not user-uploaded
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	backupPath := path + ".bak"
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", err
	}
	return backupPath, nil
}

// newKubeMergeCmd merges one or more kubeconfig files into the target.
func newKubeMergeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "merge <file...>",
		Short: "Merge kubeconfig files into the target",
		Long: `Merge one or more kubeconfig files into the target kubeconfig.
Existing entries with the same name are overwritten by the source.

Examples:
  cfgctl kube merge cluster1.yaml
  cfgctl kube merge cluster1.yaml cluster2.yaml
  cfgctl kube merge --kubeconfig /tmp/merged.yaml cluster1.yaml cluster2.yaml`,
		Args: cobra.MinimumNArgs(1),
		RunE: runKubeMerge,
	}
}

func runKubeMerge(_ *cobra.Command, args []string) error {
	targetPath := resolveKubeconfig()
	if targetPath == "" {
		return errors.New("cannot determine kubeconfig path")
	}

	target, err := loadKubeconfig(targetPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		target = api.NewConfig()
	}

	if err := maybeBackup(targetPath); err != nil {
		return err
	}

	for _, file := range args {
		source, err := loadKubeconfig(file)
		if err != nil {
			return fmt.Errorf("load %s: %w", file, err)
		}

		mergeKubeconfigEntries(target, source)
		fmt.Printf("  %s %s (%d contexts)\n", labelStyle.Render("Merged:"), pathStyle.Render(file), len(source.Contexts))
	}

	if err := writeKubeconfig(targetPath, target); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}

	fmt.Printf("  %s %s\n", labelStyle.Render("Written:"), pathStyle.Render(targetPath))
	return nil
}

func mergeKubeconfigEntries(target, source *api.Config) {
	for name, cluster := range source.Clusters {
		target.Clusters[name] = cluster
	}
	for name, authInfo := range source.AuthInfos {
		target.AuthInfos[name] = authInfo
	}
	for name, ctx := range source.Contexts {
		target.Contexts[name] = ctx
	}
	if source.CurrentContext != "" && target.CurrentContext == "" {
		target.CurrentContext = source.CurrentContext
	}
}

func maybeBackup(path string) error {
	if noBackup {
		return nil
	}
	bp, err := backupKubeconfig(path)
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	if bp != "" {
		fmt.Printf("  %s %s\n", labelStyle.Render("Backup:"), pathStyle.Render(bp))
	}
	return nil
}

// newKubeDeleteCmd removes contexts and orphaned cluster/authinfo entries.
func newKubeDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <context...>",
		Short: "Delete contexts from kubeconfig",
		Long: `Delete one or more contexts and their orphaned cluster/authinfo entries.
A cluster or user entry is removed only if no remaining context references it.

Examples:
  cfgctl kube delete my-context
  cfgctl kube delete ctx1 ctx2 ctx3`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			targetPath := resolveKubeconfig()
			if targetPath == "" {
				return errors.New("cannot determine kubeconfig path")
			}

			config, err := loadKubeconfig(targetPath)
			if err != nil {
				return err
			}

			if err := maybeBackup(targetPath); err != nil {
				return err
			}

			for _, name := range args {
				ctx, ok := config.Contexts[name]
				if !ok {
					return fmt.Errorf("context %q not found", name)
				}

				clusterName := ctx.Cluster
				userName := ctx.AuthInfo
				delete(config.Contexts, name)

				if config.CurrentContext == name {
					config.CurrentContext = ""
				}

				if !clusterReferenced(config, clusterName, "") {
					delete(config.Clusters, clusterName)
				}
				if !userReferenced(config, userName, "") {
					delete(config.AuthInfos, userName)
				}

				fmt.Printf("  %s %s\n", labelStyle.Render("Deleted:"), sectionStyle.Render(name))
			}

			if err := writeKubeconfig(targetPath, config); err != nil {
				return fmt.Errorf("write kubeconfig: %w", err)
			}
			return nil
		},
	}
}

// newKubeRenameCmd renames a context and its associated entries.
func newKubeRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old-name> <new-name>",
		Short: "Rename a kubeconfig context",
		Long: `Rename a context and its associated cluster and user entries.

Examples:
  cfgctl kube rename old-context new-context`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			oldName, newName := args[0], args[1]

			targetPath := resolveKubeconfig()
			if targetPath == "" {
				return errors.New("cannot determine kubeconfig path")
			}

			config, err := loadKubeconfig(targetPath)
			if err != nil {
				return err
			}

			ctx, ok := config.Contexts[oldName]
			if !ok {
				return fmt.Errorf("context %q not found", oldName)
			}
			if _, exists := config.Contexts[newName]; exists {
				return fmt.Errorf("context %q already exists", newName)
			}

			if err := maybeBackup(targetPath); err != nil {
				return err
			}

			oldCluster := ctx.Cluster
			oldUser := ctx.AuthInfo

			// Check sharing before moving the context entry.
			renameCluster := oldCluster == oldName && !clusterReferenced(config, oldCluster, oldName)
			renameUser := oldUser == oldName && !userReferenced(config, oldUser, oldName)

			config.Contexts[newName] = ctx
			delete(config.Contexts, oldName)

			if config.CurrentContext == oldName {
				config.CurrentContext = newName
			}

			if cluster, ok := config.Clusters[oldCluster]; ok && renameCluster {
				config.Clusters[newName] = cluster
				delete(config.Clusters, oldCluster)
				ctx.Cluster = newName
			}

			if authInfo, ok := config.AuthInfos[oldUser]; ok && renameUser {
				config.AuthInfos[newName] = authInfo
				delete(config.AuthInfos, oldUser)
				ctx.AuthInfo = newName
			}

			if err := writeKubeconfig(targetPath, config); err != nil {
				return fmt.Errorf("write kubeconfig: %w", err)
			}

			fmt.Printf("  %s %s → %s\n", labelStyle.Render("Renamed:"), sectionStyle.Render(oldName), sectionStyle.Render(newName))
			return nil
		},
	}
}

// newKubeExportCmd exports a single context to a standalone kubeconfig.
func newKubeExportCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "export <context>",
		Short: "Export a context to a standalone kubeconfig",
		Long: `Extract a single context and its associated cluster and user entries
into a standalone kubeconfig file.

Output goes to stdout by default, or to a file with --output.

Examples:
  cfgctl kube export my-context
  cfgctl kube export my-context --output cluster.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			contextName := args[0]

			sourcePath := resolveKubeconfig()
			if sourcePath == "" {
				return errors.New("cannot determine kubeconfig path")
			}

			config, err := loadKubeconfig(sourcePath)
			if err != nil {
				return err
			}

			ctx, ok := config.Contexts[contextName]
			if !ok {
				return fmt.Errorf("context %q not found", contextName)
			}

			exported := api.NewConfig()
			exported.CurrentContext = contextName
			exported.Contexts[contextName] = ctx

			cluster, ok := config.Clusters[ctx.Cluster]
			if !ok {
				return fmt.Errorf("cluster %q referenced by context %q not found", ctx.Cluster, contextName)
			}
			exported.Clusters[ctx.Cluster] = cluster

			authInfo, ok := config.AuthInfos[ctx.AuthInfo]
			if !ok {
				return fmt.Errorf("user %q referenced by context %q not found", ctx.AuthInfo, contextName)
			}
			exported.AuthInfos[ctx.AuthInfo] = authInfo

			content, err := clientcmd.Write(*exported)
			if err != nil {
				return fmt.Errorf("serialize kubeconfig: %w", err)
			}

			if output == "" {
				fmt.Print(string(content))
				return nil
			}

			if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
				return fmt.Errorf("create directory: %w", err)
			}
			if err := os.WriteFile(output, content, 0600); err != nil {
				return fmt.Errorf("write file: %w", err)
			}

			fmt.Printf("  %s %s → %s\n", labelStyle.Render("Exported:"), sectionStyle.Render(contextName), pathStyle.Render(output))
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file (default: stdout)")
	return cmd
}

// clusterReferenced reports whether any context other than exceptCtx references
// the named cluster. Pass exceptCtx="" to consider all contexts.
func clusterReferenced(config *api.Config, clusterName, exceptCtx string) bool {
	for name, ctx := range config.Contexts {
		if name != exceptCtx && ctx.Cluster == clusterName {
			return true
		}
	}
	return false
}

// userReferenced reports whether any context other than exceptCtx references the
// named user. Pass exceptCtx="" to consider all contexts.
func userReferenced(config *api.Config, userName, exceptCtx string) bool {
	for name, ctx := range config.Contexts {
		if name != exceptCtx && ctx.AuthInfo == userName {
			return true
		}
	}
	return false
}
