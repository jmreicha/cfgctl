// Package aws provides AWS configuration management.
package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmreicha/cfgctl/internal/core"
)

// ProviderName is the unique identifier for the AWS provider.
const ProviderName = "aws"

var errProviderConfigNil = errors.New("aws provider configuration is nil")

// Provider implements the core.Provider interface for AWS configuration management.
type Provider struct {
	*core.BaseProvider
	config   *Config
	discover discoverProfilesFunc
}

type discoverProfilesFunc func(context.Context, *Config) ([]DiscoveredProfile, error)

func defaultDiscoverProfiles(ctx context.Context, cfg *Config) ([]DiscoveredProfile, error) {
	return DiscoverProfiles(ctx, cfg, nil)
}

// NewProvider creates a new AWS provider instance with the given configuration.
func NewProvider(config *Config) *Provider {
	if config == nil {
		config = DefaultConfig()
	}

	return &Provider{
		BaseProvider: core.NewBaseProvider(ProviderName, config),
		config:       config,
		discover:     defaultDiscoverProfiles,
	}
}

// Name returns the unique identifier for this provider.
func (p *Provider) Name() string {
	return ProviderName
}

// Generate creates the configuration files for this provider.
func (p *Provider) Generate(ctx context.Context, opts *core.GenerateOptions) (*core.Result, error) {
	result := core.NewResult(p.Name())

	if err := p.applyGenerateOptions(opts); err != nil {
		return nil, err
	}

	if !p.config.Enabled {
		result.Warnings = append(result.Warnings, "aws provider is disabled")
		return result, nil
	}

	if p.discover == nil {
		p.discover = defaultDiscoverProfiles
	}

	outputPath := p.config.ConfigPath
	if core.CheckExistingOutput(outputPath, opts, result) {
		return result, nil
	}

	core.UpdateGenerateStatus(opts, "Discovering AWS SSO profiles...")

	profiles, err := p.discover(ctx, p.config)
	if err != nil {
		if !errors.Is(err, errSSOLoginRequired) {
			return nil, err
		}
		core.UpdateGenerateStatus(opts, "SSO login required, authenticating...")
		if loginErr := runSSOLogin(ctx, p.config); loginErr != nil {
			return nil, fmt.Errorf("auto sso login: %w", loginErr)
		}
		profiles, err = p.discover(ctx, p.config)
		if err != nil {
			return nil, err
		}
	}

	core.UpdateGenerateStatus(opts, fmt.Sprintf("Found %d profiles, writing config...", len(profiles)))

	finalContent, _, err := buildConfigContent(p.config, outputPath, profiles, result, opts)
	if err != nil {
		return nil, err
	}

	credentialsEnabled, credentialsPath, credentialsContent, err := buildCredentialsContent(p.config, profiles, result)
	if err != nil {
		return nil, err
	}

	if opts != nil && opts.DryRun {
		applyDryRunMetadata(result, outputPath, finalContent, credentialsEnabled, credentialsPath, credentialsContent)
		return result, nil
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	credentialsWriteAllowed := allowCredentialsWrite(credentialsEnabled, credentialsPath, opts, result)
	if err := writeConfigFile(outputPath, finalContent); err != nil {
		return nil, err
	}

	result.FilesCreated = append(result.FilesCreated, outputPath)
	result.Metadata["discovered_profiles"] = len(profiles)

	if credentialsEnabled && credentialsWriteAllowed {
		if err := writeCredentialsFile(credentialsPath, credentialsContent); err != nil {
			return nil, err
		}
		result.FilesCreated = append(result.FilesCreated, credentialsPath)
	}

	return result, nil
}

func (p *Provider) applyGenerateOptions(opts *core.GenerateOptions) error {
	if p.config == nil {
		return errProviderConfigNil
	}

	if opts != nil && opts.Config != nil {
		cfg, ok := opts.Config.(*Config)
		if !ok {
			return errors.New("aws config has unexpected type")
		}
		p.config = cfg
	}

	return p.config.Validate()
}

func buildConfigContent(cfg *Config, outputPath string, profiles []DiscoveredProfile, result *core.Result, opts *core.GenerateOptions) (string, []string, error) {
	configContent, generatedNames, warnings, err := BuildGeneratedConfigContent(cfg, profiles)
	if err != nil {
		return "", nil, err
	}
	result.Warnings = append(result.Warnings, warnings...)

	finalContent := configContent
	if cfg.Prune && (opts == nil || !opts.Replace) {
		mergedContent, err := mergeConfigContent(outputPath, configContent, generatedNames, cfg.MarkerKey, cfg.SSO.SessionName)
		if err != nil {
			return "", nil, err
		}
		finalContent = mergedContent
	}

	return finalContent, generatedNames, nil
}

func buildCredentialsContent(cfg *Config, profiles []DiscoveredProfile, result *core.Result) (bool, string, string, error) {
	credentialsEnabled := cfg.GenerateCredentials && cfg.UseCredentialProcess
	credentialsPath := cfg.CredentialsPath
	credentialsContent := ""
	if cfg.GenerateCredentials && !cfg.UseCredentialProcess {
		result.Warnings = append(result.Warnings, "credentials generation disabled: use_credential_process is false")
	}
	if credentialsEnabled {
		content, credentialProfiles, warnings, err := BuildCredentialProcessContent(cfg, profiles)
		if err != nil {
			return false, "", "", err
		}
		result.Warnings = append(result.Warnings, warnings...)
		credentialsContent = content
		result.Metadata["credential_profiles"] = len(credentialProfiles)
	}
	return credentialsEnabled, credentialsPath, credentialsContent, nil
}

func applyDryRunMetadata(result *core.Result, outputPath, finalContent string, credentialsEnabled bool, credentialsPath, credentialsContent string) {
	result.Warnings = append(result.Warnings, "dry-run mode: no files were actually created")
	result.Metadata["config_path"] = outputPath
	result.Metadata["config_content"] = finalContent
	result.FilesCreated = append(result.FilesCreated, outputPath)
	if credentialsEnabled {
		result.Metadata["credentials_path"] = credentialsPath
		result.Metadata["credentials_content"] = credentialsContent
		result.FilesCreated = append(result.FilesCreated, credentialsPath)
	}
}

func allowCredentialsWrite(credentialsEnabled bool, credentialsPath string, opts *core.GenerateOptions, result *core.Result) bool {
	if !credentialsEnabled {
		return false
	}
	if _, err := os.Stat(credentialsPath); err == nil && opts != nil && !opts.Force {
		result.FilesSkipped = append(result.FilesSkipped, credentialsPath)
		result.Warnings = append(result.Warnings, "credentials file exists, use --force to overwrite")
		return false
	}
	return true
}

func writeConfigFile(outputPath, content string) error {
	if err := os.WriteFile(outputPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

func writeCredentialsFile(credentialsPath, content string) error {
	if err := os.MkdirAll(filepath.Dir(credentialsPath), 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	if err := os.WriteFile(credentialsPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}
	return nil
}
