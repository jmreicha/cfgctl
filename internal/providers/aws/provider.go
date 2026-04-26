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
		config:   config,
		discover: defaultDiscoverProfiles,
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
	if checkExistingOutput(outputPath, opts, result) {
		return result, nil
	}

	profiles, err := p.discover(ctx, p.config)
	if err != nil {
		if !errors.Is(err, errSSOLoginRequired) {
			return nil, err
		}
		if loginErr := runSSOLogin(ctx, p.config); loginErr != nil {
			return nil, fmt.Errorf("auto sso login: %w", loginErr)
		}
		profiles, err = p.discover(ctx, p.config)
		if err != nil {
			return nil, err
		}
	}

	finalContent, _, err := buildConfigContent(p.config, outputPath, profiles, result)
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

func checkExistingOutput(outputPath string, opts *core.GenerateOptions, result *core.Result) bool {
	if _, err := os.Stat(outputPath); err == nil && opts != nil && !opts.Force && !opts.DryRun {
		result.FilesSkipped = append(result.FilesSkipped, outputPath)
		result.Warnings = append(result.Warnings, "config file exists, use --force to overwrite")
		return true
	}
	return false
}

func buildConfigContent(cfg *Config, outputPath string, profiles []DiscoveredProfile, result *core.Result) (string, []string, error) {
	configContent, generatedNames, warnings, err := BuildGeneratedConfigContent(cfg, profiles)
	if err != nil {
		return "", nil, err
	}
	result.Warnings = append(result.Warnings, warnings...)

	finalContent := configContent
	if cfg.Prune {
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

// Backup creates a backup of existing configuration files.
func (p *Provider) Backup(_ context.Context) (string, error) {
	if p.config == nil {
		return "", nil
	}
	return core.BackupFile(p.config.ConfigPath)
}

// NeedsBackup reports whether a backup should be created before generation.
func (p *Provider) NeedsBackup(opts *core.GenerateOptions) (bool, error) {
	if p.config == nil {
		return false, nil
	}
	if opts != nil && opts.Config != nil {
		cfg, ok := opts.Config.(*Config)
		if !ok {
			return false, errors.New("aws config has unexpected type")
		}
		p.config = cfg
	}
	if opts != nil && opts.DryRun {
		return false, nil
	}
	if !p.config.Enabled {
		return false, nil
	}
	if opts != nil && (opts.DryRun || !opts.Force) {
		if _, err := os.Stat(p.config.ConfigPath); err == nil {
			return false, nil
		}
	}
	return true, nil
}

// Restore recovers configuration from a backup.
func (p *Provider) Restore(_ context.Context, _ string) error {
	return errors.New("restore not yet implemented for aws provider")
}

// Clean removes all configuration files generated by this provider.
func (p *Provider) Clean(_ context.Context) error {
	if p.config == nil {
		return errProviderConfigNil
	}

	if p.config.ConfigPath == "" {
		return nil
	}

	path, err := normalizeConfigPath(p.config.ConfigPath)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove aws config: %w", err)
	}

	return nil
}
