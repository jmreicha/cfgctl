package aws

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
	core.RegisterProviderConfigFactory("aws", func(raw map[string]interface{}) (core.ProviderConfig, error) {
		return ConfigFromMap(raw)
	})
}

const (
	defaultMarkerKey       = "sso_auto_populated"
	defaultProfileTemplate = "{{ .AccountName }}/{{ .RoleName }}"
	defaultDemoRegion      = "us-east-1"
	defaultDemoStartURL    = "https://example.awsapps.com/start"
	defaultSSOScopes       = "sso:account:access"
	defaultSSOSessionName  = "cfgctl"
)

var (
	errCredentialsPathEmpty = errors.New("credentials path cannot be empty")
	errConfigPathEmpty      = errors.New("config path cannot be empty")
	errSSORegionEmpty       = errors.New("sso region cannot be empty")
	errSSOStartURLEmpty     = errors.New("sso start url cannot be empty")
	errTokenCachePathsEmpty = errors.New("token cache paths cannot be empty")
)

// Config represents AWS provider-specific configuration.
type Config struct {
	// Enabled indicates whether this provider should be active.
	Enabled bool `yaml:"enabled"`

	// Demo enables fake data without AWS calls.
	Demo bool `yaml:"-"`

	// ConfigPath is the output path for the AWS config file.
	ConfigPath string `yaml:"config_path"`

	// CredentialsPath is the output path for the AWS credentials file.
	CredentialsPath string `yaml:"credentials_path"`

	// GenerateCredentials enables credentials file generation.
	GenerateCredentials bool `yaml:"generate_credentials"`

	// MarkerKey tags generated profiles for pruning.
	MarkerKey string `yaml:"marker_key"`

	// Prune removes stale generated profiles.
	Prune bool `yaml:"prune"`

	// ProfilePrefix is prepended to generated profile names.
	ProfilePrefix string `yaml:"profile_prefix"`

	// ProfileTemplate is the template used for profile names.
	ProfileTemplate string `yaml:"profile_template"`
	// Roles limits discovery to matching role names.
	Roles []string `yaml:"roles"`

	// SSO contains shared SSO configuration.
	SSO SSOConfig `yaml:"sso"`

	// TokenCachePaths lists cache locations for SSO tokens.
	TokenCachePaths []string `yaml:"token_cache_paths"`

	// RoleChains define cross-account role assumptions.
	RoleChains []RoleChain `yaml:"role_chains"`

	// UseCredentialProcess configures profiles to use credential_process.
	UseCredentialProcess bool `yaml:"use_credential_process"`
}

// SSOConfig represents shared SSO configuration.
type SSOConfig struct {
	Region             string `yaml:"region"`
	RegistrationScopes string `yaml:"registration_scopes"`
	SessionName        string `yaml:"session_name"`
	StartURL           string `yaml:"start_url"`
}

// RoleChain defines a cross-account role assumption profile.
type RoleChain struct {
	Name          string `yaml:"name"`
	Region        string `yaml:"region,omitempty"`
	RoleARN       string `yaml:"role_arn"`
	SourceProfile string `yaml:"source_profile"`
}

// ConfigFromMap builds a typed configuration from a raw provider map.
func ConfigFromMap(raw map[string]interface{}) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to encode aws config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to decode aws config: %w", err)
	}

	if cfg.TokenCachePaths == nil {
		cfg.TokenCachePaths = defaultTokenCachePaths()
	}

	if cfg.ConfigPath == "" {
		cfg.ConfigPath = defaultConfigPath()
	}

	if cfg.CredentialsPath == "" {
		cfg.CredentialsPath = defaultCredentialsPath()
	}

	if cfg.ProfileTemplate == "" {
		cfg.ProfileTemplate = defaultProfileTemplate
	}

	if strings.TrimSpace(cfg.MarkerKey) == "" {
		cfg.MarkerKey = defaultMarkerKey
	}

	if cfg.SSO.RegistrationScopes == "" {
		cfg.SSO.RegistrationScopes = defaultSSOScopes
	}
	if cfg.SSO.SessionName == "" {
		cfg.SSO.SessionName = defaultSSOSessionName
	}

	return cfg, nil
}

// DefaultConfig returns the default AWS provider configuration.
func DefaultConfig() *Config {
	return &Config{
		ConfigPath:           defaultConfigPath(),
		CredentialsPath:      defaultCredentialsPath(),
		Demo:                 false,
		Enabled:              true,
		GenerateCredentials:  false,
		MarkerKey:            defaultMarkerKey,
		ProfilePrefix:        "",
		ProfileTemplate:      defaultProfileTemplate,
		Prune:                false,
		Roles:                []string{},
		RoleChains:           []RoleChain{},
		SSO:                  defaultSSOConfig(),
		TokenCachePaths:      defaultTokenCachePaths(),
		UseCredentialProcess: false,
	}
}

// Validate checks if the provider configuration is valid.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("aws config is nil")
	}

	if !c.Demo {
		if strings.TrimSpace(c.SSO.StartURL) == "" {
			return errSSOStartURLEmpty
		}

		if strings.TrimSpace(c.SSO.Region) == "" {
			return errSSORegionEmpty
		}

		if len(c.TokenCachePaths) == 0 {
			return errTokenCachePathsEmpty
		}
	}

	configPath, err := normalizeConfigPath(c.ConfigPath)
	if err != nil {
		return err
	}

	credentialsPath := ""
	if c.GenerateCredentials {
		credentialsPath, err = normalizeCredentialsPath(c.CredentialsPath)
		if err != nil {
			return err
		}
	}
	normalized := make([]string, 0, len(c.TokenCachePaths))
	for _, path := range c.TokenCachePaths {
		normalizedPath, err := normalizePath(path)
		if err != nil {
			return err
		}
		normalized = append(normalized, normalizedPath)
	}

	c.SSO.Region = strings.TrimSpace(c.SSO.Region)
	c.SSO.StartURL = strings.TrimSpace(c.SSO.StartURL)
	if c.Demo {
		if c.SSO.Region == "" {
			c.SSO.Region = defaultDemoRegion
		}
		if c.SSO.StartURL == "" {
			c.SSO.StartURL = defaultDemoStartURL
		}
	}
	if c.SSO.SessionName == "" {
		c.SSO.SessionName = defaultSSOSessionName
	}
	if strings.TrimSpace(c.SSO.RegistrationScopes) == "" {
		c.SSO.RegistrationScopes = defaultSSOScopes
	}
	if strings.TrimSpace(c.ProfileTemplate) == "" {
		c.ProfileTemplate = defaultProfileTemplate
	}
	if strings.TrimSpace(c.MarkerKey) == "" {
		c.MarkerKey = defaultMarkerKey
	}
	c.ConfigPath = configPath
	if c.GenerateCredentials {
		c.CredentialsPath = credentialsPath
	}
	c.TokenCachePaths = normalized

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

func defaultSSOConfig() SSOConfig {
	return SSOConfig{
		Region:             "",
		RegistrationScopes: defaultSSOScopes,
		SessionName:        defaultSSOSessionName,
		StartURL:           "",
	}
}

func defaultTokenCachePaths() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return []string{}
	}

	return []string{
		filepath.Join(home, ".aws", "sso", "cache"),
		filepath.Join(home, ".granted"),
	}
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	return filepath.Join(home, ".aws", "config")
}

func defaultCredentialsPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	return filepath.Join(home, ".aws", "credentials")
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

func normalizePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errTokenCachePathsEmpty
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

func normalizeConfigPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errConfigPathEmpty
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

func normalizeCredentialsPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errCredentialsPathEmpty
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
