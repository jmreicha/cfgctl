// Package cli provides the command-line interface for cfgctl.
package cli

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/jmreicha/cfgctl/internal/core"
	"github.com/jmreicha/cfgctl/internal/providers/aws"
	"github.com/jmreicha/cfgctl/internal/providers/granted"
	"github.com/jmreicha/cfgctl/internal/providers/kubernetes"
	"github.com/jmreicha/cfgctl/internal/providers/ssh"
	"github.com/jmreicha/cfgctl/internal/providers/steampipe"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// Global flags.
	cfgFile  string
	dryRun   bool
	noBackup bool
	debug    bool
	verbose  bool

	// Kubernetes generate flags.
	kubeMerge     bool
	kubeMergeOnly bool
	eksRegions    string
	eksRoles      string

	// SSH generate flag (moved off root).
	sshConfigPath string

	// AWS generate flags.
	awsCredentialProcess bool
	awsCredentials       bool
	awsDemo              bool
	awsPrefix            string
	awsPrune             bool
	awsRoleFilters       string
	awsSSOStartURL       string
	awsSSORegion         string
	awsTemplate          string

	// Steampipe generate flags.
	steampipeIgnoreErrors bool
	steampipeRegions      string

	// Shared components.
	registry      *core.Registry
	backupManager *core.BackupManager
	config        *core.Config
	engine        *core.Engine
	logger        *slog.Logger
)

// NewRootCmd creates the root command for cfgctl.
func NewRootCmd(version string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "cfgctl",
		Short: "A tool for creating and managing configurations",

		Version:      version,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			viper.SetEnvPrefix("CFGCTL")
			viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
			viper.AutomaticEnv()

			_ = viper.BindPFlag("config", cmd.Root().PersistentFlags().Lookup("config"))
			_ = viper.BindPFlag("dry-run", cmd.Root().PersistentFlags().Lookup("dry-run"))
			_ = viper.BindPFlag("debug", cmd.Root().PersistentFlags().Lookup("debug"))
			_ = viper.BindPFlag("verbose", cmd.Root().PersistentFlags().Lookup("verbose"))
			_ = viper.BindPFlag("no-backup", cmd.Root().PersistentFlags().Lookup("no-backup"))

			// Apply env var overrides where the flag was not explicitly set.
			if !cmd.Root().PersistentFlags().Changed("config") {
				cfgFile = viper.GetString("config")
			}
			if !cmd.Root().PersistentFlags().Changed("dry-run") {
				dryRun = viper.GetBool("dry-run")
			}
			if !cmd.Root().PersistentFlags().Changed("debug") {
				debug = viper.GetBool("debug")
			}
			if !cmd.Root().PersistentFlags().Changed("verbose") {
				verbose = viper.GetBool("verbose")
			}
			if !cmd.Root().PersistentFlags().Changed("no-backup") {
				noBackup = viper.GetBool("no-backup")
			}

			return initializeComponents()
		},
	}

	helpTemplate := strings.ReplaceAll(rootCmd.HelpTemplate(), "Available Commands:", "Commands:")
	usageTemplate := strings.ReplaceAll(rootCmd.UsageTemplate(), "Available Commands:", "Commands:")
	rootCmd.SetHelpTemplate(helpTemplate)
	rootCmd.SetUsageTemplate(usageTemplate)

	// Global flags — ssh-config-path is on generate, not here.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: search in standard locations)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "simulate actions without making changes")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose provider output")
	rootCmd.PersistentFlags().BoolVar(&noBackup, "no-backup", false, "skip backup creation before generation")

	// Add subcommands
	rootCmd.AddCommand(newCheckCmd())
	rootCmd.AddCommand(newCleanCmd())
	rootCmd.AddCommand(newGenerateCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newValidateCmd())

	return rootCmd
}

// initializeComponents sets up the core components needed by all commands.
func initializeComponents() error {
	logLevel := slog.LevelError
	if verbose {
		logLevel = slog.LevelInfo
	}
	if debug {
		logLevel = slog.LevelDebug
	}
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	var err error
	config, err = core.LoadConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if config == nil {
		config = core.NewConfig()
	}

	if verbose {
		config.Verbose = true
	}
	if dryRun {
		config.DryRun = true
	}
	if noBackup {
		config.NoBackup = true
	}

	registry = core.NewRegistry()
	backupManager = core.NewBackupManager("")
	engine = core.NewEngine(registry, backupManager, config, logger)

	var sshConfig *ssh.Config
	providerConfig := config.GetProviderConfig(ssh.ProviderName)
	if providerConfig == nil {
		sshConfig = ssh.DefaultConfig()
	} else {
		typedConfig, ok := providerConfig.(*ssh.Config)
		if !ok {
			return fmt.Errorf("ssh provider config has unexpected type %T", providerConfig)
		}
		sshConfig = typedConfig
	}
	if sshConfigPath != "" {
		sshConfig.ConfigPath = sshConfigPath
	}
	if err := registry.Register(ssh.NewProvider(sshConfig)); err != nil {
		return fmt.Errorf("failed to register ssh provider: %w", err)
	}

	var grantedConfig *granted.Config
	providerConfig = config.GetProviderConfig(granted.ProviderName)
	if providerConfig == nil {
		grantedConfig = granted.DefaultConfig()
	} else {
		typedConfig, ok := providerConfig.(*granted.Config)
		if !ok {
			return fmt.Errorf("granted provider config has unexpected type %T", providerConfig)
		}
		grantedConfig = typedConfig
	}
	if err := registry.Register(granted.NewProvider(grantedConfig)); err != nil {
		return fmt.Errorf("failed to register granted provider: %w", err)
	}

	var awsConfig *aws.Config
	providerConfig = config.GetProviderConfig(aws.ProviderName)
	if providerConfig == nil {
		awsConfig = aws.DefaultConfig()
		config.SetProviderConfig(aws.ProviderName, awsConfig)
	} else {
		typedConfig, ok := providerConfig.(*aws.Config)
		if !ok {
			return fmt.Errorf("aws provider config has unexpected type %T", providerConfig)
		}
		awsConfig = typedConfig
	}
	applyAWSCLIOverrides(awsConfig)
	if err := registry.Register(aws.NewProvider(awsConfig)); err != nil {
		return fmt.Errorf("failed to register aws provider: %w", err)
	}

	var kubernetesConfig *kubernetes.Config
	providerConfig = config.GetProviderConfig(kubernetes.ProviderName)
	if providerConfig == nil {
		kubernetesConfig = kubernetes.DefaultConfig()
		config.SetProviderConfig(kubernetes.ProviderName, kubernetesConfig)
	} else {
		typedConfig, ok := providerConfig.(*kubernetes.Config)
		if !ok {
			return fmt.Errorf("kubernetes provider config has unexpected type %T", providerConfig)
		}
		kubernetesConfig = typedConfig
	}
	applyKubernetesCLIOverrides(kubernetesConfig)
	if err := registry.Register(kubernetes.NewProvider(kubernetesConfig, kubernetes.WithLogger(logger))); err != nil {
		return fmt.Errorf("failed to register kubernetes provider: %w", err)
	}

	var steampipeConfig *steampipe.Config
	providerConfig = config.GetProviderConfig(steampipe.ProviderName)
	if providerConfig == nil {
		steampipeConfig = steampipe.DefaultConfig()
		config.SetProviderConfig(steampipe.ProviderName, steampipeConfig)
	} else {
		typedConfig, ok := providerConfig.(*steampipe.Config)
		if !ok {
			return fmt.Errorf("steampipe provider config has unexpected type %T", providerConfig)
		}
		steampipeConfig = typedConfig
	}
	applySteampipeCLIOverrides(steampipeConfig)
	if err := registry.Register(steampipe.NewProvider(steampipeConfig)); err != nil {
		return fmt.Errorf("failed to register steampipe provider: %w", err)
	}

	return nil
}

func applyKubernetesCLIOverrides(cfg *kubernetes.Config) {
	if cfg == nil {
		return
	}

	if regions := parseCSVFlag(eksRegions); len(regions) > 0 {
		cfg.AWS.Regions = regions
	}

	if roles := parseCSVFlag(eksRoles); len(roles) > 0 {
		cfg.AWS.Roles = roles
	}

	if kubeMergeOnly {
		cfg.MergeOnly = true
		cfg.MergeEnabled = true
	} else if kubeMerge {
		cfg.MergeEnabled = true
	}
}

func applyAWSCLIOverrides(cfg *aws.Config) {
	if cfg == nil {
		return
	}

	if awsCredentialProcess {
		cfg.UseCredentialProcess = true
	}

	if awsCredentials {
		cfg.GenerateCredentials = true
	}

	if awsDemo {
		cfg.Demo = true
	}

	if strings.TrimSpace(awsPrefix) != "" {
		cfg.ProfilePrefix = strings.TrimSpace(awsPrefix)
	}

	if awsPrune {
		cfg.Prune = true
	}

	if strings.TrimSpace(awsTemplate) != "" {
		cfg.ProfileTemplate = strings.TrimSpace(awsTemplate)
	}

	if roles := parseCSVFlag(awsRoleFilters); len(roles) > 0 {
		cfg.Roles = roles
	}

	if strings.TrimSpace(awsSSOStartURL) != "" {
		cfg.SSO.StartURL = strings.TrimSpace(awsSSOStartURL)
	}

	if strings.TrimSpace(awsSSORegion) != "" {
		cfg.SSO.Region = strings.TrimSpace(awsSSORegion)
	}
}

func applySteampipeCLIOverrides(cfg *steampipe.Config) {
	if cfg == nil {
		return
	}

	if steampipeIgnoreErrors && len(cfg.IgnoreErrorCodes) == 0 {
		cfg.IgnoreErrorCodes = steampipe.DefaultIgnoreErrorCodes
	}

	if regions := parseCSVFlag(steampipeRegions); len(regions) > 0 {
		cfg.Regions = regions
	}
}

func parseCSVFlag(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	set := make(map[string]struct{})
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}

	if len(set) == 0 {
		return nil
	}

	values := make([]string, 0, len(set))
	for item := range set {
		values = append(values, item)
	}
	sort.Strings(values)
	return values
}
