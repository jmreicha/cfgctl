// Package ssh provides an SSH configuration provider implementation.
package ssh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmreicha/cfgctl/internal/core"
	"github.com/kevinburke/ssh_config"
)

// ProviderName is the unique identifier for the SSH provider.
const ProviderName = "ssh"

var (
	errConfigPathEmpty   = errors.New("config path cannot be empty")
	errProviderConfigNil = errors.New("ssh provider configuration is nil")
)

// Provider implements the core.Provider interface for SSH configuration management.
// It handles generation, backup, restoration, and cleanup of SSH configuration files.
type Provider struct {
	*core.BaseProvider
	config *Config
}

func (c *Config) normalizedConfigPath() (string, error) {
	if c.ConfigPath == "" {
		return "", errConfigPathEmpty
	}

	configPath := filepath.Clean(os.ExpandEnv(c.ConfigPath))
	if !filepath.IsAbs(configPath) {
		return "", fmt.Errorf("config path must be absolute: %s", configPath)
	}

	return configPath, nil
}

// Validate checks if the host configuration is valid.
func (c *HostConfig) Validate() error {
	if c.Host == "" {
		return errors.New("host pattern cannot be empty")
	}
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535: %d", c.Port)
	}
	if c.IdentityAgent != "" {
		identityAgent := filepath.Clean(os.ExpandEnv(c.IdentityAgent))
		if !filepath.IsAbs(identityAgent) {
			return fmt.Errorf("identity agent must be an absolute path: %s", identityAgent)
		}
		c.IdentityAgent = identityAgent
	}

	if c.IdentityFile != "" {
		identityFile := filepath.Clean(os.ExpandEnv(c.IdentityFile))
		if !filepath.IsAbs(identityFile) {
			return fmt.Errorf("identity file must be an absolute path: %s", identityFile)
		}
		c.IdentityFile = identityFile
	}

	return nil
}

// Validate checks if the provider configuration is valid.
func (c *Config) Validate() error {
	configPath, err := c.normalizedConfigPath()
	if err != nil {
		return err
	}

	for i := range c.Hosts {
		host := &c.Hosts[i]
		if err := host.Validate(); err != nil {
			return fmt.Errorf("host %d: %w", i, err)
		}
	}

	c.ConfigPath = configPath

	return nil
}

// NewProvider creates a new SSH provider instance with the given configuration.
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
func (p *Provider) Name() string {
	return ProviderName
}

// Generate creates the configuration files for this provider.
// Returns a Result containing details about what was generated.
func (p *Provider) Generate(_ context.Context, opts *core.GenerateOptions) (*core.Result, error) {
	result := core.NewResult(p.Name())

	if p.config == nil {
		return nil, errProviderConfigNil
	}

	if opts != nil && opts.Config != nil {
		cfg, ok := opts.Config.(*Config)
		if !ok {
			return nil, errors.New("ssh config has unexpected type")
		}
		p.config = cfg
	}

	if err := p.config.Validate(); err != nil {
		return nil, err
	}

	if !p.config.Enabled {
		result.Warnings = append(result.Warnings, "ssh provider is disabled")
		return result, nil
	}

	// Parse history files if enabled
	if p.config.ParseHistory {
		if err := p.mergeHistoryHosts(result); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to parse history: %v", err))
		}
	}

	if len(p.config.Hosts) == 0 && len(p.config.GlobalOptions) == 0 {
		result.Warnings = append(result.Warnings, "no hosts configured")
		return result, nil
	}

	configPath := filepath.Join(p.config.ConfigPath, "config")

	if core.CheckExistingOutput(configPath, opts, result) {
		return result, nil
	}

	// Parse existing config or create new one
	var cfg *ssh_config.Config
	_, statErr := os.Stat(configPath)
	if statErr == nil {
		var err error
		cfg, err = ParseConfig(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse existing config: %w", err)
		}
	} else {
		cfg = &ssh_config.Config{}
	}

	existingHosts := FindHostsByPatterns(cfg)

	if len(p.config.GlobalOptions) > 0 {
		if _, err := UpsertGlobalOptions(cfg, p.config.GlobalOptions); err != nil {
			return nil, fmt.Errorf("failed to merge global options: %w", err)
		}
	}

	// Add or update hosts from configuration
	hostsAdded, hostsUpdated := p.upsertHosts(cfg, existingHosts, result)

	if opts.DryRun {
		result.Warnings = append(result.Warnings, "dry-run mode: no files were actually created")
		result.Metadata["hosts_to_add"] = hostsAdded
		result.Metadata["hosts_to_update"] = hostsUpdated
		result.FilesCreated = append(result.FilesCreated, configPath)
		return result, nil
	}

	// Ensure config directory exists
	if err := os.MkdirAll(p.config.ConfigPath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config
	if err := WriteConfig(cfg, configPath); err != nil {
		return nil, fmt.Errorf("failed to write config: %w", err)
	}

	result.FilesCreated = append(result.FilesCreated, configPath)
	result.Metadata["hosts_added"] = hostsAdded
	result.Metadata["hosts_updated"] = hostsUpdated

	return result, nil
}

// mergeHistoryHosts parses shell history and merges discovered hosts into the configuration.
func (p *Provider) mergeHistoryHosts(result *core.Result) error {
	historyCommands, err := ParseHistoryFiles()
	if err != nil {
		return err
	}

	// Convert history commands to host configs and add to config
	added := 0
	for _, cmd := range historyCommands {
		hostConfig := cmd.ConvertToHostConfig()
		// Only add if not already in configuration
		exists := false
		for _, existing := range p.config.Hosts {
			if existing.Host == hostConfig.Host {
				exists = true
				break
			}
		}
		if !exists {
			p.config.Hosts = append(p.config.Hosts, hostConfig)
			added++
		}
	}

	if added > 0 {
		result.Metadata["hosts_from_history"] = added
	}

	return nil
}

// upsertHosts adds or updates hosts in the SSH configuration.
func (p *Provider) upsertHosts(cfg *ssh_config.Config, existingHosts map[string]*ssh_config.Host, result *core.Result) (int, int) {
	hostsAdded := 0
	hostsUpdated := 0

	for _, host := range p.config.Hosts {
		existingHost := existingHosts[host.Host]
		if existingHost != nil {
			if err := UpdateHost(cfg, host); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("failed to update host %s: %v", host.Host, err))
				continue
			}
			hostsUpdated++
		} else {
			if err := AddHost(cfg, host); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("failed to add host %s: %v", host.Host, err))
				continue
			}
			hostsAdded++
		}
	}

	return hostsAdded, hostsUpdated
}
