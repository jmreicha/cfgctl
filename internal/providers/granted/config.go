package granted

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmreicha/cfgctl/internal/core"
	"gopkg.in/yaml.v3"
)

//nolint:gochecknoinits // Required for registering provider config factory
func init() {
	core.RegisterProviderConfigFactory("granted", func(raw map[string]interface{}) (core.ProviderConfig, error) {
		return ConfigFromMap(raw)
	})
}

const (
	defaultBrowserValue = "STDOUT"
)

var (
	errConfigPathEmpty = errors.New("config path cannot be empty")
)

// Config represents Granted provider-specific configuration.
type Config struct {
	// Enabled indicates whether this provider should be active.
	Enabled bool `yaml:"enabled"`

	// ConfigPath is the output path for the generated Granted config.
	ConfigPath string `yaml:"config_path"`

	// CredentialProcessAutoLogin enables automatic login for credential_process.
	CredentialProcessAutoLogin bool `yaml:"credential_process_auto_login"`

	// CustomBrowserPath is the path to a custom browser executable.
	CustomBrowserPath string `yaml:"custom_browser_path"`

	// CustomSSOBrowserPath is the path to a custom SSO browser executable.
	CustomSSOBrowserPath string `yaml:"custom_sso_browser_path"`

	// DefaultBrowser controls where credentials are output (e.g., "STDOUT", "FIREFOX").
	DefaultBrowser string `yaml:"default_browser"`

	// DisableUsageTips disables usage tips in Granted output.
	DisableUsageTips bool `yaml:"disable_usage_tips"`

	// ExportCredentialSuffix is a suffix added to exported credential environment variables.
	ExportCredentialSuffix string `yaml:"export_credential_suffix"`

	// Ordering controls how profiles are ordered in selection.
	Ordering string `yaml:"ordering"`
}

// ConfigFromMap builds a typed configuration from a raw provider map.
func ConfigFromMap(raw map[string]interface{}) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to encode granted config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to decode granted config: %w", err)
	}

	if cfg.ConfigPath == "" {
		cfg.ConfigPath = defaultConfigPath()
	}

	if cfg.DefaultBrowser == "" {
		cfg.DefaultBrowser = defaultBrowser()
	}

	return cfg, nil
}

// DefaultConfig returns the default Granted provider configuration.
func DefaultConfig() *Config {
	return &Config{
		ConfigPath:                 defaultConfigPath(),
		CredentialProcessAutoLogin: true,
		CustomBrowserPath:          "",
		CustomSSOBrowserPath:       "",
		DefaultBrowser:             defaultBrowser(),
		DisableUsageTips:           true,
		Enabled:                    true,
		ExportCredentialSuffix:     "",
		Ordering:                   "",
	}
}

// Validate checks if the provider configuration is valid.
func (c *Config) Validate() error {
	configPath, err := normalizePath(c.ConfigPath, errConfigPathEmpty)
	if err != nil {
		return err
	}

	c.ConfigPath = configPath

	return nil
}

// GetConfigPath returns the path to the primary config file.
func (c *Config) GetConfigPath() string { return c.ConfigPath }

// IsEnabled reports whether the provider is enabled.
func (c *Config) IsEnabled() bool {
	if c == nil {
		return false
	}

	return c.Enabled
}

func defaultBrowser() string {
	return defaultBrowserValue
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	return filepath.Join(home, ".granted", "config")
}

func expandHomeDir(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("failed to resolve home directory")
	}

	if path == "~" {
		return home, nil
	}

	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:]), nil
	}

	return path, nil
}

func normalizePath(path string, emptyErr error) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", emptyErr
	}

	expanded, err := expandHomeDir(os.ExpandEnv(path))
	if err != nil {
		return "", err
	}

	expanded = filepath.Clean(expanded)
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("path must be absolute: %s", path)
	}

	return expanded, nil
}
