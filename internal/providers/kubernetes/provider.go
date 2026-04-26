// Package kubernetes provides a Kubernetes configuration provider implementation.
package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmreicha/cfgctl/internal/core"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// ProviderName is the unique identifier for the Kubernetes provider.
const ProviderName = "kubernetes"

var errProviderConfigNil = errors.New("kubernetes provider configuration is nil")

// Provider implements the core.Provider interface for Kubernetes configuration management.
type Provider struct {
	config *Config
	logger *slog.Logger
}

// NewProvider creates a new Kubernetes provider instance with the given configuration.
func NewProvider(config *Config, opts ...ProviderOption) *Provider {
	if config == nil {
		config = DefaultConfig()
	}

	p := &Provider{config: config, logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ProviderOption configures a Provider.
type ProviderOption func(*Provider)

// WithLogger sets the logger for the provider.
func WithLogger(logger *slog.Logger) ProviderOption {
	return func(p *Provider) {
		if logger != nil {
			p.logger = logger
		}
	}
}

// Name returns the unique identifier for this provider.
func (p *Provider) Name() string {
	return ProviderName
}

// Validate checks if all prerequisites for this provider are met.
func (p *Provider) Validate(_ context.Context) error {
	if p.config == nil {
		return errProviderConfigNil
	}

	if !p.config.Enabled {
		return nil
	}

	return p.config.Validate()
}

// Generate creates the configuration files for this provider.
func (p *Provider) Generate(ctx context.Context, opts *core.GenerateOptions) (*core.Result, error) {
	result := &core.Result{
		Provider:     p.Name(),
		FilesCreated: []string{},
		FilesSkipped: []string{},
		Warnings:     []string{},
		Metadata:     make(map[string]interface{}),
	}

	if err := p.validateAndPrepare(opts); err != nil {
		return nil, err
	}

	if !p.config.Enabled {
		result.Warnings = append(result.Warnings, "kubernetes provider is disabled")
		return result, nil
	}

	p.logger.Debug("starting kubernetes generation",
		"config_path", p.config.ConfigPath,
		"merge_only", p.config.MergeOnly,
		"merge_enabled", p.config.MergeEnabled,
	)

	discovered, discoveryWarnings, err := p.discoverClusters(ctx)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, discoveryWarnings...)

	for _, c := range discovered {
		p.logger.Debug("discovered cluster",
			"profile", c.Profile,
			"region", c.Region,
			"cluster", c.Name,
			"auth_mode", c.AuthMode,
		)
	}

	mergeConfig, mergeFiles, err := p.buildKubeconfig(discovered)
	if err != nil {
		return nil, err
	}

	if mergeConfig == nil {
		result.Warnings = append(result.Warnings, "no kubeconfig data generated")
		return result, nil
	}

	if len(mergeFiles) > 0 {
		for _, f := range mergeFiles {
			p.logger.Debug("merging kubeconfig file", "path", f)
		}
		result.Metadata["merge_files"] = mergeFiles
	}

	for name := range mergeConfig.Contexts {
		p.logger.Debug("generating context", "name", name)
	}

	// Populate summary metadata.
	result.Metadata["discovered_clusters"] = len(discovered)
	result.Metadata["regions"] = p.config.AWS.Regions

	authModes := map[string]struct{}{}
	for _, c := range discovered {
		if c.AuthMode != "" {
			authModes[c.AuthMode] = struct{}{}
		} else {
			authModes["aws-cli"] = struct{}{}
		}
	}
	if len(authModes) > 0 {
		modes := make([]string, 0, len(authModes))
		for m := range authModes {
			modes = append(modes, m)
		}
		result.Metadata["auth_mode"] = strings.Join(modes, ", ")
	}

	if opts != nil && opts.DryRun {
		return p.handleDryRun(result, discovered, mergeConfig), nil
	}

	outputPath := p.config.ConfigPath
	p.logger.Debug("writing kubeconfig", "path", outputPath)
	if err := p.writeKubeconfig(outputPath, mergeConfig, opts, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (p *Provider) validateAndPrepare(opts *core.GenerateOptions) error {
	if p.config == nil {
		return errProviderConfigNil
	}

	if opts != nil && opts.Config != nil {
		cfg, ok := opts.Config.(*Config)
		if !ok {
			return errors.New("kubernetes config has unexpected type")
		}
		p.config = cfg
	}

	return p.config.Validate()
}

func (p *Provider) discoverClusters(ctx context.Context) ([]DiscoveredCluster, []string, error) {
	if p.config.MergeOnly {
		return nil, nil, nil
	}

	return DiscoverEKSClusters(ctx, p.config, nil, p.logger)
}

func (p *Provider) buildKubeconfig(discovered []DiscoveredCluster) (*api.Config, []string, error) {
	var discoveredConfig *api.Config
	if len(discovered) > 0 {
		var err error
		discoveredConfig, err = BuildKubeconfig(discovered, p.config.NamingPattern)
		if err != nil {
			return nil, nil, err
		}
	}

	manualConfig, err := buildManualKubeconfig(p.config.ManualConfigs)
	if err != nil && !errors.Is(err, errManualConfigsEmpty) {
		return nil, nil, err
	}

	if !p.config.MergeEnabled && !p.config.MergeOnly {
		if discoveredConfig == nil && manualConfig == nil {
			return nil, nil, nil
		}

		merged := newKubeconfig()
		mergeInto(merged, discoveredConfig)
		mergeInto(merged, manualConfig)
		return merged, nil, nil
	}

	outputPath := p.config.ConfigPath
	mergeConfig, mergeFiles, err := MergeKubeconfigs(outputPath, p.config.Merge, discoveredConfig)
	if err != nil {
		return nil, nil, err
	}

	mergeInto(mergeConfig, manualConfig)
	return mergeConfig, mergeFiles, nil
}

func (p *Provider) handleDryRun(result *core.Result, discovered []DiscoveredCluster, mergeConfig *api.Config) *core.Result {
	result.Warnings = append(result.Warnings, "dry-run mode: no files were actually created")
	result.Metadata["discovered_clusters"] = len(discovered)
	result.Metadata["dry_run_output"] = p.config.ConfigPath
	if mergeConfig != nil {
		result.Metadata["dry_run_kubeconfig"] = mergeConfig
	}
	result.FilesCreated = append(result.FilesCreated, p.config.ConfigPath)
	return result
}

func (p *Provider) writeKubeconfig(outputPath string, mergeConfig *api.Config, opts *core.GenerateOptions, result *core.Result) error {
	if outputPath == "" {
		return errors.New("kubernetes config path is empty")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
		return fmt.Errorf("create kubeconfig directory: %w", err)
	}

	// Check if output file exists and skip if force is not set
	if _, err := os.Stat(outputPath); err == nil && !opts.Force {
		result.FilesSkipped = append(result.FilesSkipped, outputPath)
		result.Warnings = append(result.Warnings, "kubeconfig already exists, use --force to overwrite")
		return nil
	}

	if err := clientcmd.WriteToFile(*mergeConfig, outputPath); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}

	result.FilesCreated = append(result.FilesCreated, outputPath)
	return nil
}

// NeedsBackup reports whether a backup should be created before generation.
func (p *Provider) NeedsBackup(opts *core.GenerateOptions) (bool, error) {
	if p.config == nil {
		return false, nil
	}
	if opts != nil && opts.Config != nil {
		cfg, ok := opts.Config.(*Config)
		if !ok {
			return false, errors.New("kubernetes config has unexpected type")
		}
		p.config = cfg
	}
	if opts != nil && opts.DryRun {
		return false, nil
	}
	if !p.config.Enabled {
		return false, nil
	}
	if opts == nil || !opts.Force {
		if _, err := os.Stat(p.config.ConfigPath); err == nil {
			return false, nil
		}
	}
	return true, nil
}

// Backup creates a backup of existing configuration files.
func (p *Provider) Backup(_ context.Context) (string, error) {
	if p.config == nil {
		return "", nil
	}
	return core.BackupFile(p.config.ConfigPath)
}

// Restore recovers configuration from a backup.
func (p *Provider) Restore(_ context.Context, _ string) error {
	return errors.New("restore not yet implemented for kubernetes provider")
}

// Clean removes all configuration files generated by this provider.
func (p *Provider) Clean(_ context.Context) error {
	if p.config == nil {
		return errProviderConfigNil
	}

	if p.config.ConfigPath == "" {
		return nil
	}

	path, err := normalizePath(p.config.ConfigPath, errConfigPathEmpty)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove kubeconfig: %w", err)
	}

	return nil
}
