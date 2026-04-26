package steampipe

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
	core.RegisterProviderConfigFactory("steampipe", func(raw map[string]interface{}) (core.ProviderConfig, error) {
		return ConfigFromMap(raw)
	})
}

const (
	defaultConnectionPrefix = "aws_"
	defaultAWSConfigPath    = ""
	defaultConfigPath       = ""
)

// DefaultIgnoreErrorCodes is the set of AWS error codes added when
// --steampipe-ignore-errors is passed on the CLI.
var DefaultIgnoreErrorCodes = []string{
	"AccessDenied",
	"AccessDeniedException",
	"AuthorizationError",
	"NotAuthorized",
	"UnauthorizedOperation",
	"UnrecognizedClientException",
}

var errConfigPathEmpty = errors.New("config_path is required")

// Config represents steampipe provider-specific configuration.
type Config struct {
	// AWSConfigPath is the source AWS config file to read profiles from.
	AWSConfigPath string `yaml:"aws_config_path"`

	// BackupDir is where backup files are written.
	BackupDir string `yaml:"backup_dir"`

	// BackupEnabled controls whether backups are created before overwriting.
	BackupEnabled bool `yaml:"backup_enabled"`

	// ConfigPath is the output path for the steampipe AWS connection config.
	ConfigPath string `yaml:"config_path"`

	// ConnectionPrefix is prepended to generated connection names.
	ConnectionPrefix string `yaml:"connection_prefix"`

	// Enabled indicates whether this provider should be active.
	Enabled bool `yaml:"enabled"`

	// ProfileRegions maps profile names to per-profile region lists.
	ProfileRegions map[string][]string `yaml:"profile_regions"`

	// Profiles limits generation to matching profile names (empty = all).
	Profiles []string `yaml:"profiles"`

	// IgnoreErrorCodes is the list of AWS error codes to silently ignore on
	// each connection. Empty by default; use --steampipe-ignore-errors or set
	// this field in config to enable.
	IgnoreErrorCodes []string `yaml:"ignore_error_codes"`

	// PreferredRoles is an ordered list of SSO role names. When an AWS account
	// has multiple profiles (e.g. account/cloudinfra and account/lytxread),
	// the first matching role in this list is used. If none match, the first
	// profile encountered is used.
	PreferredRoles []string `yaml:"preferred_roles"`

	// Regions is the default region list for each connection.
	Regions []string `yaml:"regions"`
}

// ConfigFromMap builds a typed configuration from a raw provider map.
func ConfigFromMap(raw map[string]interface{}) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to encode steampipe config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to decode steampipe config: %w", err)
	}

	if cfg.AWSConfigPath == "" {
		cfg.AWSConfigPath = defaultAWSConfigPath
	}

	if cfg.ConfigPath == "" {
		cfg.ConfigPath = defaultSteampipeConfigPath()
	}

	if cfg.BackupDir == "" {
		cfg.BackupDir = filepath.Dir(cfg.ConfigPath)
	}

	if cfg.ConnectionPrefix == "" {
		cfg.ConnectionPrefix = defaultConnectionPrefix
	}

	if len(cfg.Regions) == 0 {
		cfg.Regions = []string{"*"}
	}

	return cfg, nil
}

// DefaultConfig returns the default steampipe provider configuration.
func DefaultConfig() *Config {
	return &Config{
		AWSConfigPath:    defaultAWSConfigPath,
		BackupDir:        "",
		BackupEnabled:    true,
		ConfigPath:       defaultSteampipeConfigPath(),
		ConnectionPrefix: defaultConnectionPrefix,
		Enabled:          true,
		IgnoreErrorCodes: []string{},
		PreferredRoles:   []string{},
		ProfileRegions:   map[string][]string{},
		Profiles:         []string{},
		Regions:          []string{"*"},
	}
}

// Validate checks if the provider configuration is valid.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("steampipe config is nil")
	}

	if strings.TrimSpace(c.ConfigPath) == "" {
		return errConfigPathEmpty
	}

	expanded, err := expandHomeDir(c.ConfigPath)
	if err != nil {
		return err
	}
	c.ConfigPath = filepath.Clean(expanded)

	if strings.TrimSpace(c.AWSConfigPath) != "" {
		expanded, err := expandHomeDir(c.AWSConfigPath)
		if err != nil {
			return err
		}
		c.AWSConfigPath = filepath.Clean(expanded)
	}

	if c.BackupDir == "" {
		c.BackupDir = filepath.Dir(c.ConfigPath)
	} else {
		expanded, err := expandHomeDir(c.BackupDir)
		if err != nil {
			return err
		}
		c.BackupDir = filepath.Clean(expanded)
	}

	if c.ConnectionPrefix == "" {
		c.ConnectionPrefix = defaultConnectionPrefix
	}

	if len(c.Regions) == 0 {
		c.Regions = []string{"*"}
	}

	if c.ProfileRegions == nil {
		c.ProfileRegions = map[string][]string{}
	}

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

func defaultSteampipeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".steampipe", "config", "aws.spc")
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
