package core

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the global configuration for cfgctl.
// It can be loaded from YAML files or set via CLI flags.
type Config struct {
	// Global settings
	Verbose  bool `yaml:"verbose"`
	DryRun   bool `yaml:"dry_run"`
	NoBackup bool `yaml:"no_backup"`

	// Providers contains provider-specific configuration.
	// The map key is the provider name.
	Providers map[string]ProviderConfig `yaml:"-"`

	// RawProviders stores provider configs from YAML until decoded.
	RawProviders map[string]map[string]interface{} `yaml:"providers"`
}

// NewConfig creates a new configuration with default values.
func NewConfig() *Config {
	return &Config{
		Verbose:      false,
		DryRun:       false,
		NoBackup:     false,
		Providers:    make(map[string]ProviderConfig),
		RawProviders: make(map[string]map[string]interface{}),
	}
}

// LoadConfig loads configuration from a YAML file.
// If the file doesn't exist, returns a default configuration.
// The precedence order is: CLI flags > YAML config > defaults.
func LoadConfig(path string) (*Config, error) {
	cfg := NewConfig()

	if path == "" {
		path = FindConfigFile()
	}

	if path == "" {
		return cfg, nil
	}

	// #nosec G304 -- config file path is from user input or searched standard locations
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := cfg.decodeProviders(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// FindConfigSearchPaths returns the ordered list of paths where cfgctl looks for a config file.
func FindConfigSearchPaths() []string {
	return []string{
		"./cfgctl.yaml",
		"./cfgctl.yml",
		filepath.Join(os.Getenv("HOME"), ".config", "cfgctl", "config.yaml"),
		filepath.Join(os.Getenv("HOME"), ".config", "cfgctl", "config.yml"),
		filepath.Join(os.Getenv("HOME"), ".cfgctl", "config.yaml"),
		filepath.Join(os.Getenv("HOME"), ".cfgctl", "config.yml"),
	}
}

// FindConfigFile searches for a cfgctl configuration file in standard locations.
// Returns an empty string if no config file is found.
func FindConfigFile() string {
	for _, path := range FindConfigSearchPaths() {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// GetProviderConfig retrieves the configuration for a specific provider.
// Returns nil if no configuration exists for the provider.
func (c *Config) GetProviderConfig(providerName string) ProviderConfig {
	if c.Providers == nil {
		return nil
	}
	return c.Providers[providerName]
}

// SetProviderConfig sets the configuration for a specific provider.
func (c *Config) SetProviderConfig(providerName string, config ProviderConfig) {
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}
	c.Providers[providerName] = config
}

// Merge merges another configuration into this one.
// Values from the other configuration take precedence.
func (c *Config) Merge(other *Config) {
	if other.Verbose {
		c.Verbose = true
	}
	if other.DryRun {
		c.DryRun = true
	}
	if other.NoBackup {
		c.NoBackup = true
	}

	if other.Providers != nil {
		if c.Providers == nil {
			c.Providers = make(map[string]ProviderConfig)
		}
		for name, providerCfg := range other.Providers {
			c.Providers[name] = providerCfg
		}
	}
}

func (c *Config) decodeProviders() error {
	if len(c.RawProviders) == 0 {
		return nil
	}

	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}

	for name, providerCfg := range c.RawProviders {
		cfg, err := ProviderConfigFromMap(name, providerCfg)
		if err != nil {
			return err
		}
		if cfg != nil {
			c.Providers[name] = cfg
		}
	}

	return nil
}
