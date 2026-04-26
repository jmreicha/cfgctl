// Package granted provides a Granted configuration provider implementation.
package granted

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmreicha/cfgctl/internal/core"
)

// ProviderName is the unique identifier for the Granted provider.
const ProviderName = "granted"

// Provider implements the core.Provider interface for Granted configuration management.
type Provider struct {
	*core.BaseProvider
	config *Config
}

// NewProvider creates a new Granted provider instance with the given configuration.
func NewProvider(config *Config) *Provider {
	if config == nil {
		config = DefaultConfig()
	}
	return &Provider{
		BaseProvider: core.NewBaseProvider(ProviderName, config),
		config:       config,
	}
}

// Name returns the unique identifier for this provider.
func (p *Provider) Name() string { return ProviderName }

// Generate creates the configuration files for this provider.
func (p *Provider) Generate(_ context.Context, opts *core.GenerateOptions) (*core.Result, error) {
	result := core.NewResult(p.Name())

	if p.config == nil {
		return nil, errors.New("granted provider configuration is nil")
	}

	if opts != nil && opts.Config != nil {
		cfg, ok := opts.Config.(*Config)
		if !ok {
			return nil, errors.New("granted config has unexpected type")
		}
		p.config = cfg
	}

	if err := p.config.Validate(); err != nil {
		return nil, err
	}

	if !p.config.Enabled {
		result.Warnings = append(result.Warnings, "granted provider is disabled")
		return result, nil
	}

	configPath := p.config.ConfigPath

	if core.CheckExistingOutput(configPath, opts, result) {
		return result, nil
	}

	configContent := p.buildConfigContent()

	if opts != nil && opts.DryRun {
		result.Warnings = append(result.Warnings, "dry-run mode: no files were actually created")
		result.Metadata["config_path"] = configPath
		result.Metadata["config_content"] = configContent
		result.FilesCreated = append(result.FilesCreated, configPath)
		return result, nil
	}

	if err := core.WriteConfigFile(configPath, []byte(configContent), 0600); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	result.FilesCreated = append(result.FilesCreated, configPath)
	return result, nil
}

// buildConfigContent generates the Granted config file content.
func (p *Provider) buildConfigContent() string {
	return fmt.Sprintf(`DefaultBrowser = %q
CustomBrowserPath = %q
CustomSSOBrowserPath = %q
Ordering = %q
ExportCredentialSuffix = %q
DisableUsageTips = %t
CredentialProcessAutoLogin = %t
`,
		p.config.DefaultBrowser,
		p.config.CustomBrowserPath,
		p.config.CustomSSOBrowserPath,
		p.config.Ordering,
		p.config.ExportCredentialSuffix,
		p.config.DisableUsageTips,
		p.config.CredentialProcessAutoLogin,
	)
}
