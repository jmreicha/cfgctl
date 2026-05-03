package kubernetes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmreicha/cfgctl/internal/core"
	"gopkg.in/yaml.v3"
)

//nolint:gochecknoinits // Required for registering provider config factory
func init() {
	core.RegisterProviderConfigFactory("kubernetes", func(raw map[string]interface{}) (core.ProviderConfig, error) {
		return ConfigFromMap(raw)
	})
}

var (
	errAWSConfigFileEmpty    = errors.New("aws config file cannot be empty")
	errAWSCredentialsEmpty   = errors.New("aws credentials file cannot be empty")
	errConfigPathEmpty       = errors.New("config path cannot be empty")
	errMergeSourceDirEmpty   = errors.New("merge source directory cannot be empty")
	errNamingPatternEmpty    = errors.New("naming pattern cannot be empty")
	errParallelWorkersBounds = errors.New("parallel workers must be greater than zero")
	errRegionsEmpty          = errors.New("aws regions cannot be empty")
	errTimeoutInvalid        = errors.New("timeout must be greater than zero")
)

// Config represents Kubernetes provider-specific configuration.
type Config struct {
	// Enabled indicates whether this provider should be active.
	Enabled bool `yaml:"enabled"`

	// ConfigPath is the output path for the generated kubeconfig.
	ConfigPath string `yaml:"config_path"`

	// AWS defines settings for EKS discovery.
	AWS AWSConfig `yaml:"aws"`

	// NamingPattern controls naming for cluster, context, and user entries.
	NamingPattern string `yaml:"naming_pattern"`

	// MergeEnabled toggles merging existing kubeconfig files.
	MergeEnabled bool `yaml:"merge_enabled"`

	// MergeOnly skips AWS discovery and merges existing kubeconfigs only.
	MergeOnly bool `yaml:"merge_only"`

	// Merge contains settings for merging existing kubeconfig files.
	Merge MergeConfig `yaml:"merge"`

	// ManualConfigs defines manual kubeconfig entries to preserve.
	ManualConfigs []ManualConfig `yaml:"manual_configs"`
}

// AWSConfig represents EKS discovery settings.
type AWSConfig struct {
	// ConfigFile is the path to the AWS config file (~/.aws/config).
	// Used for profile resolution and SSO credential loading.
	ConfigFile string `yaml:"config_file"`

	// CredentialsFile is the path to the AWS credentials file.
	//
	// Deprecated: prefer ConfigFile for SSO-based authentication.
	CredentialsFile string `yaml:"credentials_file"`

	// Roles filters profiles to only those matching the given role names.
	// Matches against the role portion of profile names (e.g. "adminaccess"
	// matches "prod/adminaccess"). Empty means all roles.
	Roles []string `yaml:"roles"`

	// Regions defines which AWS regions to scan for clusters.
	Regions []string `yaml:"regions"`

	// ParallelWorkers controls how many profile/region scans run in parallel.
	ParallelWorkers int `yaml:"parallel_workers"`

	// Timeout is the AWS call timeout for discovery.
	Timeout time.Duration `yaml:"timeout"`
}

// MergeConfig defines settings for merging existing kubeconfig files.
type MergeConfig struct {
	// SourceDir is the directory to scan for kubeconfig files.
	SourceDir string `yaml:"source_dir"`

	// IncludePatterns defines glob patterns to include.
	IncludePatterns []string `yaml:"include_patterns"`

	// ExcludePatterns defines glob patterns to exclude.
	ExcludePatterns []string `yaml:"exclude_patterns"`
}

// DiscoveredCluster represents a cluster discovered from AWS.
type DiscoveredCluster struct {
	Profile  string
	Region   string
	Name     string
	Endpoint string
	CAData   []byte
	AuthMode string // authModeAWSVault or "" (default: aws cli with SSO/profile)
}

// ManualConfig represents a manually managed kubeconfig entry.
type ManualConfig struct {
	// Name is a stable identifier for the manual config.
	Name string `yaml:"name"`

	// ClusterName is the kubeconfig cluster entry name.
	ClusterName string `yaml:"cluster"`

	// ContextName is the kubeconfig context entry name.
	ContextName string `yaml:"context"`

	// UserName is the kubeconfig user entry name.
	UserName string `yaml:"user"`

	// ClusterEndpoint is the API server endpoint.
	ClusterEndpoint string `yaml:"cluster_endpoint"`

	// ClusterCAData is the base64-encoded cluster CA data.
	ClusterCAData string `yaml:"cluster_ca_data"`

	// ClusterCAFile is a file path to the cluster CA certificate.
	ClusterCAFile string `yaml:"cluster_ca_file"`

	// AuthInfo defines the user authentication details.
	AuthInfo ManualAuthInfo `yaml:"auth_info"`

	// ContextSettings defines the context-specific settings.
	ContextSettings ManualContext `yaml:"context_settings"`
}

// ManualAuthInfo represents user authentication details for manual configs.
type ManualAuthInfo struct {
	// ClientCertificateData is the base64-encoded client certificate data.
	ClientCertificateData string `yaml:"client_certificate_data"`

	// ClientCertificateFile is a file path to the client certificate.
	ClientCertificateFile string `yaml:"client_certificate_file"`

	// ClientKeyData is the base64-encoded client key data.
	ClientKeyData string `yaml:"client_key_data"`

	// ClientKeyFile is a file path to the client key.
	ClientKeyFile string `yaml:"client_key_file"`

	// Token is a bearer token for authentication.
	Token string `yaml:"token"`

	// Username is the basic auth username.
	Username string `yaml:"username"`

	// Password is the basic auth password.
	Password string `yaml:"password"`

	// Exec defines an exec-based auth command.
	Exec ManualExecConfig `yaml:"exec"`
}

// ManualExecConfig represents exec auth configuration for manual configs.
type ManualExecConfig struct {
	// APIVersion is the exec API version.
	APIVersion string `yaml:"api_version"`

	// Command is the executable to run.
	Command string `yaml:"command"`

	// Args are the command arguments.
	Args []string `yaml:"args"`

	// Env sets environment variables for the command.
	Env map[string]string `yaml:"env"`
}

// ManualContext represents context settings for manual configs.
type ManualContext struct {
	// Namespace is the default namespace for the context.
	Namespace string `yaml:"namespace"`
}

// ConfigFromMap builds a typed configuration from a raw provider map.
func ConfigFromMap(raw map[string]interface{}) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to encode kubernetes config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to decode kubernetes config: %w", err)
	}

	if cfg.ConfigPath == "" {
		cfg.ConfigPath = defaultConfigPath()
	}

	if cfg.AWS.ConfigFile == "" {
		cfg.AWS.ConfigFile = defaultAWSConfigFile()
	}

	if cfg.AWS.CredentialsFile == "" {
		cfg.AWS.CredentialsFile = defaultCredentialsFile()
	}

	if cfg.AWS.Regions == nil {
		cfg.AWS.Regions = defaultRegions()
	}

	if cfg.AWS.ParallelWorkers == 0 {
		cfg.AWS.ParallelWorkers = defaultParallelWorkers()
	}

	if cfg.AWS.Timeout == 0 {
		cfg.AWS.Timeout = defaultTimeout()
	}

	if cfg.NamingPattern == "" {
		cfg.NamingPattern = defaultNamingPattern()
	}

	if cfg.Merge.SourceDir == "" {
		cfg.Merge.SourceDir = defaultMergeSourceDir()
	}

	if cfg.Merge.IncludePatterns == nil {
		cfg.Merge.IncludePatterns = defaultIncludePatterns()
	}

	if cfg.Merge.ExcludePatterns == nil {
		cfg.Merge.ExcludePatterns = defaultExcludePatterns()
	}

	if cfg.ManualConfigs == nil {
		cfg.ManualConfigs = []ManualConfig{}
	}

	return cfg, nil
}

// DefaultConfig returns the default Kubernetes provider configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:       true,
		ConfigPath:    defaultConfigPath(),
		AWS:           defaultAWSConfig(),
		NamingPattern: defaultNamingPattern(),
		MergeEnabled:  false,
		MergeOnly:     false,
		Merge:         defaultMergeConfig(),
		ManualConfigs: []ManualConfig{},
	}
}

// Validate checks if the provider configuration is valid.
func (c *Config) Validate() error {
	configPath, err := normalizePath(c.ConfigPath, errConfigPathEmpty)
	if err != nil {
		return err
	}

	mergeSourceDir, err := normalizePath(c.Merge.SourceDir, errMergeSourceDirEmpty)
	if err != nil {
		return err
	}

	// Enable merge when MergeOnly is requested.
	if c.MergeOnly {
		c.MergeEnabled = true
	}

	// AWS validation is only required when not in MergeOnly mode.
	awsValidationRequired := !c.MergeOnly

	if awsValidationRequired {
		configFile, err := normalizePath(c.AWS.ConfigFile, errAWSConfigFileEmpty)
		if err != nil {
			return err
		}

		if len(c.AWS.Regions) == 0 {
			return errRegionsEmpty
		}

		if c.AWS.ParallelWorkers <= 0 {
			return errParallelWorkersBounds
		}

		if c.AWS.Timeout <= 0 {
			return errTimeoutInvalid
		}

		c.AWS.ConfigFile = configFile

		// CredentialsFile is optional; normalize if provided.
		if strings.TrimSpace(c.AWS.CredentialsFile) != "" {
			credentialsFile, err := normalizePath(c.AWS.CredentialsFile, errAWSCredentialsEmpty)
			if err != nil {
				return err
			}
			c.AWS.CredentialsFile = credentialsFile
		}
	}

	if strings.TrimSpace(c.NamingPattern) == "" {
		return errNamingPatternEmpty
	}

	c.ConfigPath = configPath
	c.Merge.SourceDir = mergeSourceDir
	c.NamingPattern = strings.TrimSpace(c.NamingPattern)

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

func defaultAWSConfig() AWSConfig {
	return AWSConfig{
		ConfigFile:      defaultAWSConfigFile(),
		CredentialsFile: defaultCredentialsFile(),
		Regions:         defaultRegions(),
		ParallelWorkers: defaultParallelWorkers(),
		Timeout:         defaultTimeout(),
	}
}

func defaultMergeConfig() MergeConfig {
	return MergeConfig{
		SourceDir:       defaultMergeSourceDir(),
		IncludePatterns: defaultIncludePatterns(),
		ExcludePatterns: defaultExcludePatterns(),
	}
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	return filepath.Join(home, ".kube", "config")
}

func defaultAWSConfigFile() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	return filepath.Join(home, ".aws", "config")
}

func defaultCredentialsFile() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	return filepath.Join(home, ".aws", "credentials")
}

func defaultRegions() []string {
	return []string{"all"}
}

func defaultParallelWorkers() int {
	return 10
}

func defaultTimeout() time.Duration {
	return 30 * time.Second
}

const defaultNamingPatternValue = "{profile}-{cluster}"

func defaultNamingPattern() string {
	return defaultNamingPatternValue
}

func defaultMergeSourceDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	return filepath.Join(home, ".kube")
}

func defaultIncludePatterns() []string {
	return []string{"*.yaml", "*.yml", "config"}
}

func defaultExcludePatterns() []string {
	return []string{"*.bak", "*.backup"}
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
