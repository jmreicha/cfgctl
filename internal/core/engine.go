package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
)

// Engine coordinates provider lifecycle: validation, backup, generation, error handling, and rollback.
type Engine struct {
	registry      *Registry
	backupManager *BackupManager
	config        *Config
	logger        *slog.Logger
	status        StatusUpdater
}

// NewEngine creates a new engine with the provided components.
func NewEngine(registry *Registry, backupManager *BackupManager, config *Config, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}

	return &Engine{
		registry:      registry,
		backupManager: backupManager,
		config:        config,
		logger:        logger,
		status:        NoopStatus{},
	}
}

// SetStatus sets the status updater for user-facing progress messages.
func (e *Engine) SetStatus(s StatusUpdater) {
	if s != nil {
		e.status = s
	}
}

// ExecuteOptions contains options for executing generation.
type ExecuteOptions struct {
	// Providers lists the specific providers to run.
	// If empty, all registered providers will be executed.
	Providers []string

	// DryRun simulates generation without making changes.
	DryRun bool

	// Force overwrites existing configurations.
	Force bool

	// NoBackup skips backup creation before generation.
	NoBackup bool

	// Verbose enables detailed logging.
	Verbose bool
}

// Execute runs the generation process for the specified providers.
// Returns a map of provider names to their results, or an error if the process fails.
//
//nolint:gocognit // Execution flow coordinates multiple phases and providers.
func (e *Engine) Execute(ctx context.Context, opts *ExecuteOptions) (map[string]*Result, error) {
	if opts == nil {
		opts = &ExecuteOptions{}
	}

	// Determine which providers to run
	providers, err := e.resolveProviders(opts.Providers)
	if err != nil {
		return nil, err
	}

	if len(providers) == 0 {
		return nil, errors.New("no providers to execute")
	}

	e.logger.Info("starting generation", "provider_count", len(providers))

	results := make(map[string]*Result)
	backups := make(map[string]string)

	// Execute each provider
	total := len(providers)
	for i, provider := range providers {
		providerName := provider.Name()
		e.logger.Info("processing provider", "provider", providerName)

		missingTools := e.providerMissingTools(providerName)
		if len(missingTools) > 0 {
			if !e.providerEnabled(providerName) {
				result := &Result{
					Provider: providerName,
					Warnings: []string{providerName + " provider is disabled"},
				}
				results[providerName] = result
				continue
			}
			result := &Result{
				Provider: providerName,
				Warnings: []string{fmt.Sprintf("%s provider disabled: missing tools %s", providerName, strings.Join(missingTools, ", "))},
			}
			results[providerName] = result
			continue
		}

		e.status.UpdateStatus(fmt.Sprintf("[%d/%d] Generating %s configuration...", i+1, total, providerName))

		// Phase 1: Validate
		if err := e.validateProvider(ctx, provider); err != nil {
			return results, fmt.Errorf("validation failed for provider %q: %w", providerName, err)
		}

		// Phase 2: Backup (unless disabled or dry-run)
		var backupPath string
		shouldBackup := !opts.NoBackup && !opts.DryRun
		if shouldBackup {
			if decider, ok := provider.(BackupDecider); ok {
				backupNeeded, err := decider.NeedsBackup(&GenerateOptions{
					DryRun:  opts.DryRun,
					Force:   opts.Force,
					Verbose: opts.Verbose,
					Config:  e.config.GetProviderConfig(providerName),
				})
				if err != nil {
					return results, fmt.Errorf("backup check failed for provider %q: %w", providerName, err)
				}
				shouldBackup = backupNeeded
			}
		}
		if shouldBackup {
			backupPath, err = e.backupProvider(ctx, provider)
			if err != nil {
				e.logger.Warn("backup failed", "provider", providerName, "error", err)
			} else if backupPath != "" {
				backups[providerName] = backupPath
				e.logger.Info("backup created", "provider", providerName, "path", backupPath)
			}
		}

		// Phase 3: Generate
		result, err := e.generateProvider(ctx, provider, opts)
		if err != nil {
			e.logger.Error("generation failed", "provider", providerName, "error", err)

			// Attempt rollback if we have a backup
			if backupPath != "" {
				e.logger.Info("attempting rollback", "provider", providerName)
				if rollbackErr := e.backupManager.Restore(backupPath); rollbackErr != nil {
					e.logger.Error("rollback failed", "provider", providerName, "error", rollbackErr)
				} else {
					e.logger.Info("rollback successful", "provider", providerName)
				}
			}

			return results, fmt.Errorf("generation failed for provider %q: %w", providerName, friendlyError(err))
		}

		result.BackupPath = backupPath
		result.DryRun = opts.DryRun
		results[providerName] = result

		e.logger.Info("provider completed", "provider", providerName, "files_created", len(result.FilesCreated))
	}

	e.logger.Info("generation complete", "provider_count", len(results))
	return results, nil
}

// ValidateAll validates all providers without generating anything.
func (e *Engine) ValidateAll(ctx context.Context) error {
	providers := e.registry.GetAll()

	for _, provider := range providers {
		if err := e.validateProvider(ctx, provider); err != nil {
			return fmt.Errorf("validation failed for provider %q: %w", provider.Name(), err)
		}
	}

	return nil
}

// CleanProvider removes all configuration files for a specific provider.
func (e *Engine) CleanProvider(ctx context.Context, providerName string) error {
	provider, err := e.registry.Get(providerName)
	if err != nil {
		return err
	}

	e.logger.Info("cleaning provider", "provider", providerName)

	if err := provider.Clean(ctx); err != nil {
		return fmt.Errorf("clean failed for provider %q: %w", providerName, err)
	}

	e.logger.Info("clean complete", "provider", providerName)
	return nil
}

// resolveProviders determines which providers to execute based on the input list.
func (e *Engine) resolveProviders(providerNames []string) ([]Provider, error) {
	uniqueNames := deduplicateStrings(providerNames)

	if len(uniqueNames) == 0 {
		// Return all registered providers
		allProviders := e.registry.GetAll()
		return reorderProvidersForDependencies(allProviders), nil
	}

	providers := make([]Provider, 0, len(uniqueNames))
	for _, name := range uniqueNames {
		provider, err := e.registry.Get(name)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}

	return reorderProvidersForDependencies(providers), nil
}

func deduplicateStrings(ss []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// reorderProvidersForDependencies ensures AWS provider runs first when present.
// Other providers (kubernetes, steampipe, etc.) depend on AWS config being generated.
func reorderProvidersForDependencies(providers []Provider) []Provider {
	if len(providers) <= 1 {
		return providers
	}

	var awsProvider Provider
	remaining := make([]Provider, 0, len(providers))

	for _, p := range providers {
		if p.Name() == "aws" {
			awsProvider = p
		} else {
			remaining = append(remaining, p)
		}
	}

	if awsProvider == nil {
		return providers
	}

	result := make([]Provider, 0, len(providers))
	result = append(result, awsProvider)
	result = append(result, remaining...)
	return result
}

// validateProvider runs validation for a single provider.
func (e *Engine) validateProvider(ctx context.Context, provider Provider) error {
	e.logger.Debug("validating provider", "provider", provider.Name())

	if err := provider.Validate(ctx); err != nil {
		return err
	}

	e.logger.Debug("validation passed", "provider", provider.Name())
	return nil
}

// backupProvider creates a backup for a provider.
// Returns empty string and nil error if no backup is needed.
func (e *Engine) backupProvider(ctx context.Context, provider Provider) (string, error) {
	e.logger.Debug("creating backup", "provider", provider.Name())

	backupPath, err := provider.Backup(ctx)
	if err != nil {
		return "", err
	}

	return backupPath, nil
}

// generateProvider runs generation for a single provider.
func (e *Engine) generateProvider(ctx context.Context, provider Provider, opts *ExecuteOptions) (*Result, error) {
	e.logger.Debug("generating configuration", "provider", provider.Name())

	// Get provider-specific configuration
	providerCfg := e.config.GetProviderConfig(provider.Name())

	genOpts := &GenerateOptions{
		DryRun:  opts.DryRun,
		Force:   opts.Force,
		Verbose: opts.Verbose,
		Config:  providerCfg,
		Status:  e.status,
	}

	result, err := provider.Generate(ctx, genOpts)
	if err != nil {
		return nil, err
	}

	result.Provider = provider.Name()
	return result, nil
}

func (e *Engine) providerMissingTools(providerName string) []string {
	switch providerName {
	case "aws":
		return MissingExecutables("aws")
	case "granted":
		return MissingExecutables("granted")
	case "kubernetes":
		return MissingExecutables("k9s", "kubectl")
	case "ssh":
		return MissingExecutables("ssh")
	case "steampipe":
		return MissingExecutables("steampipe")
	default:
		return nil
	}
}

func (e *Engine) providerEnabled(providerName string) bool {
	providerCfg := e.config.GetProviderConfig(providerName)
	if providerCfg == nil {
		return true
	}

	if enabledCfg, ok := providerCfg.(EnabledProviderConfig); ok {
		return enabledCfg.IsEnabled()
	}

	return true
}

// friendlyError wraps known infrastructure errors with short, actionable messages.
// The original error is preserved in the chain for --debug output.
func friendlyError(err error) error {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return fmt.Errorf("DNS lookup failed for %q (check your network/VPN connection): %w", dnsErr.Name, err)
	}

	var netOpErr *net.OpError
	if errors.As(err, &netOpErr) {
		return fmt.Errorf("network error (check your network/VPN connection): %w", err)
	}

	return err
}
