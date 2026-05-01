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
	*core.BaseProvider
	config *Config
	logger *slog.Logger
}

// NewProvider creates a new Kubernetes provider instance with the given configuration.
func NewProvider(config *Config, opts ...ProviderOption) *Provider {
	if config == nil {
		config = DefaultConfig()
	}

	p := &Provider{
		BaseProvider: core.NewBaseProvider(ProviderName, config),
		config:       config,
		logger:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
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

// Generate creates the configuration files for this provider.
func (p *Provider) Generate(ctx context.Context, opts *core.GenerateOptions) (*core.Result, error) {
	result := core.NewResult(p.Name())

	if err := p.validateAndPrepare(opts); err != nil {
		return nil, err
	}

	if !p.config.Enabled {
		result.Warnings = append(result.Warnings, "kubernetes provider is disabled")
		return result, nil
	}

	if core.CheckExistingOutput(p.config.ConfigPath, opts, result) {
		return result, nil
	}

	p.logger.Debug("starting kubernetes generation",
		"config_path", p.config.ConfigPath,
		"merge_only", p.config.MergeOnly,
		"merge_enabled", p.config.MergeEnabled,
	)

	if !p.config.MergeOnly {
		regions := strings.Join(p.config.AWS.Regions, ", ")
		if regions == "" {
			regions = "auto-detected"
		}
		core.UpdateGenerateStatus(opts, fmt.Sprintf("Discovering EKS clusters (regions: %s)...", regions))
	}

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

	if len(discovered) > 0 {
		core.UpdateGenerateStatus(opts, fmt.Sprintf("Found %d clusters, building kubeconfig...", len(discovered)))
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
	if err := p.writeKubeconfig(outputPath, mergeConfig, result); err != nil {
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

func (p *Provider) writeKubeconfig(outputPath string, mergeConfig *api.Config, result *core.Result) error {
	if outputPath == "" {
		return errors.New("kubernetes config path is empty")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
		return fmt.Errorf("create kubeconfig directory: %w", err)
	}

	if err := clientcmd.WriteToFile(*mergeConfig, outputPath); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}

	result.FilesCreated = append(result.FilesCreated, outputPath)
	return nil
}
