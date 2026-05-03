package kubernetes

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jmreicha/cfgctl/internal/core"
)

func TestConfigFromMapDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := ConfigFromMap(map[string]interface{}{})
	if err != nil {
		t.Fatalf("ConfigFromMap failed: %v", err)
	}

	if cfg.ConfigPath != filepath.Join(home, ".kube", "config") {
		t.Errorf("ConfigPath = %q", cfg.ConfigPath)
	}

	if cfg.AWS.ConfigFile != filepath.Join(home, ".aws", "config") {
		t.Errorf("ConfigFile = %q", cfg.AWS.ConfigFile)
	}

	if cfg.AWS.CredentialsFile != filepath.Join(home, ".aws", "credentials") {
		t.Errorf("CredentialsFile = %q", cfg.AWS.CredentialsFile)
	}

	if !reflect.DeepEqual(cfg.AWS.Regions, defaultRegions()) {
		t.Errorf("Regions = %v", cfg.AWS.Regions)
	}

	if cfg.AWS.ParallelWorkers != defaultParallelWorkers() {
		t.Errorf("ParallelWorkers = %d", cfg.AWS.ParallelWorkers)
	}

	if cfg.AWS.Timeout != defaultTimeout() {
		t.Errorf("Timeout = %v", cfg.AWS.Timeout)
	}

	if cfg.NamingPattern != defaultNamingPattern() {
		t.Errorf("NamingPattern = %q", cfg.NamingPattern)
	}

	if cfg.Merge.SourceDir != filepath.Join(home, ".kube") {
		t.Errorf("Merge.SourceDir = %q", cfg.Merge.SourceDir)
	}

	if !reflect.DeepEqual(cfg.Merge.IncludePatterns, defaultIncludePatterns()) {
		t.Errorf("IncludePatterns = %v", cfg.Merge.IncludePatterns)
	}

	if !reflect.DeepEqual(cfg.Merge.ExcludePatterns, defaultExcludePatterns()) {
		t.Errorf("ExcludePatterns = %v", cfg.Merge.ExcludePatterns)
	}

	if len(cfg.ManualConfigs) != 0 {
		t.Errorf("ManualConfigs = %v", cfg.ManualConfigs)
	}
}

func TestConfigFromMapOverrides(t *testing.T) {
	raw := map[string]interface{}{
		"config_path":    "/custom/kubeconfig",
		"naming_pattern": "{profile}-{region}-{cluster}",
		"aws": map[string]interface{}{
			"config_file":      "/custom/aws-config",
			"credentials_file": "/custom/creds",
			"regions":          []string{"us-west-2"},
			"parallel_workers": 4,
			"timeout":          "45s",
		},
		"merge": map[string]interface{}{
			"source_dir":       "/custom/merge",
			"include_patterns": []string{"*.yaml"},
			"exclude_patterns": []string{"*.bak"},
		},
		"manual_configs": []map[string]interface{}{
			{
				"name":             "docker-desktop",
				"cluster":          "docker-desktop",
				"context":          "docker-desktop",
				"user":             "docker-desktop",
				"cluster_endpoint": "https://kubernetes.docker.internal:6443",
				"cluster_ca_data":  "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
				"auth_info": map[string]interface{}{
					"token": "example-token",
					"exec": map[string]interface{}{
						"api_version": "client.authentication.k8s.io/v1beta1",
						"command":     "aws",
						"args":        []string{"eks", "get-token", "--cluster-name", "demo"},
						"env": map[string]interface{}{
							"AWS_PROFILE": "demo",
						},
					},
				},
				"context_settings": map[string]interface{}{
					"namespace": "default",
				},
			},
		},
	}

	cfg, err := ConfigFromMap(raw)
	if err != nil {
		t.Fatalf("ConfigFromMap failed: %v", err)
	}

	if cfg.ConfigPath != "/custom/kubeconfig" {
		t.Errorf("ConfigPath = %q", cfg.ConfigPath)
	}

	if cfg.NamingPattern != "{profile}-{region}-{cluster}" {
		t.Errorf("NamingPattern = %q", cfg.NamingPattern)
	}

	if cfg.AWS.ConfigFile != "/custom/aws-config" {
		t.Errorf("ConfigFile = %q", cfg.AWS.ConfigFile)
	}

	if cfg.AWS.CredentialsFile != "/custom/creds" {
		t.Errorf("CredentialsFile = %q", cfg.AWS.CredentialsFile)
	}

	if !reflect.DeepEqual(cfg.AWS.Regions, []string{"us-west-2"}) {
		t.Errorf("Regions = %v", cfg.AWS.Regions)
	}

	if cfg.AWS.ParallelWorkers != 4 {
		t.Errorf("ParallelWorkers = %d", cfg.AWS.ParallelWorkers)
	}

	if cfg.AWS.Timeout != 45*time.Second {
		t.Errorf("Timeout = %v", cfg.AWS.Timeout)
	}

	if cfg.Merge.SourceDir != "/custom/merge" {
		t.Errorf("Merge.SourceDir = %q", cfg.Merge.SourceDir)
	}

	if !reflect.DeepEqual(cfg.Merge.IncludePatterns, []string{"*.yaml"}) {
		t.Errorf("IncludePatterns = %v", cfg.Merge.IncludePatterns)
	}

	if !reflect.DeepEqual(cfg.Merge.ExcludePatterns, []string{"*.bak"}) {
		t.Errorf("ExcludePatterns = %v", cfg.Merge.ExcludePatterns)
	}

	expectedManualConfigs := []ManualConfig{
		{
			Name:            "docker-desktop",
			ClusterName:     "docker-desktop",
			ContextName:     "docker-desktop",
			UserName:        "docker-desktop",
			ClusterEndpoint: "https://kubernetes.docker.internal:6443",
			ClusterCAData:   "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
			AuthInfo: ManualAuthInfo{
				Token: "example-token",
				Exec: ManualExecConfig{
					APIVersion: "client.authentication.k8s.io/v1beta1",
					Command:    "aws",
					Args:       []string{"eks", "get-token", "--cluster-name", "demo"},
					Env: map[string]string{
						"AWS_PROFILE": "demo",
					},
				},
			},
			ContextSettings: ManualContext{
				Namespace: "default",
			},
		},
	}

	if !reflect.DeepEqual(cfg.ManualConfigs, expectedManualConfigs) {
		t.Errorf("ManualConfigs = %v", cfg.ManualConfigs)
	}
}

func TestConfigValidate(t *testing.T) {
	baseDir := t.TempDir()

	cfg := &Config{
		Enabled:    true,
		ConfigPath: filepath.Join(baseDir, "kube", "config"),
		AWS: AWSConfig{
			ConfigFile:      filepath.Join(baseDir, "aws", "config"),
			CredentialsFile: filepath.Join(baseDir, "aws", "credentials"),
			Regions:         []string{"us-west-2"},
			ParallelWorkers: 2,
			Timeout:         5 * time.Second,
		},
		NamingPattern: " {profile}-{cluster} ",
		MergeOnly:     true,
		Merge: MergeConfig{
			SourceDir:       filepath.Join(baseDir, "kube"),
			IncludePatterns: []string{"*.yaml"},
			ExcludePatterns: []string{"*.bak"},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if !cfg.MergeEnabled {
		t.Error("expected MergeEnabled to be true when MergeOnly is set")
	}

	if cfg.NamingPattern != defaultNamingPatternValue {
		t.Errorf("NamingPattern = %q", cfg.NamingPattern)
	}
}

func TestConfigFromMapNil(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := ConfigFromMap(nil)
	if err != nil {
		t.Fatalf("ConfigFromMap failed: %v", err)
	}

	if cfg == nil {
		t.Error("expected non-nil config")
	}
}

func TestConfigFromMapInvalidYAML(t *testing.T) {
	raw := map[string]interface{}{
		"enabled": "not-a-bool",
	}

	_, err := ConfigFromMap(raw)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestProviderConfigFactoryRegistration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := core.ProviderConfigFromMap("kubernetes", map[string]interface{}{})
	if err != nil {
		t.Fatalf("ProviderConfigFromMap failed: %v", err)
	}

	kubeCfg, ok := cfg.(*Config)
	if !ok {
		t.Fatalf("expected *Config, got %T", cfg)
	}

	if !kubeCfg.Enabled {
		t.Error("expected enabled to be true by default")
	}
}

func TestDefaultConfigPathNoHome(t *testing.T) {
	t.Setenv("HOME", "")

	path := defaultConfigPath()
	if path != "" {
		t.Errorf("defaultConfigPath() = %q, want empty string", path)
	}
}

func TestDefaultAWSConfigFileNoHome(t *testing.T) {
	t.Setenv("HOME", "")

	path := defaultAWSConfigFile()
	if path != "" {
		t.Errorf("defaultAWSConfigFile() = %q, want empty string", path)
	}
}

func TestDefaultCredentialsFileNoHome(t *testing.T) {
	t.Setenv("HOME", "")

	path := defaultCredentialsFile()
	if path != "" {
		t.Errorf("defaultCredentialsFile() = %q, want empty string", path)
	}
}

func TestDefaultMergeSourceDirNoHome(t *testing.T) {
	t.Setenv("HOME", "")

	path := defaultMergeSourceDir()
	if path != "" {
		t.Errorf("defaultMergeSourceDir() = %q, want empty string", path)
	}
}

func TestConfigValidateErrors(t *testing.T) {
	baseDir := t.TempDir()
	base := &Config{
		Enabled:    true,
		ConfigPath: filepath.Join(baseDir, "kube", "config"),
		AWS: AWSConfig{
			ConfigFile:      filepath.Join(baseDir, "aws", "config"),
			CredentialsFile: filepath.Join(baseDir, "aws", "credentials"),
			Regions:         []string{"us-west-2"},
			ParallelWorkers: 1,
			Timeout:         5 * time.Second,
		},
		NamingPattern: defaultNamingPattern(),
		Merge: MergeConfig{
			SourceDir: filepath.Join(baseDir, "kube"),
		},
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr error
	}{
		{
			name: "empty config path",
			mutate: func(cfg *Config) {
				cfg.ConfigPath = ""
			},
			wantErr: errConfigPathEmpty,
		},
		{
			name: "empty merge source dir",
			mutate: func(cfg *Config) {
				cfg.Merge.SourceDir = ""
			},
			wantErr: errMergeSourceDirEmpty,
		},
		{
			name: "relative config path",
			mutate: func(cfg *Config) {
				cfg.ConfigPath = "relative/config"
			},
		},
		{
			name: "empty regions",
			mutate: func(cfg *Config) {
				cfg.AWS.Regions = nil
			},
			wantErr: errRegionsEmpty,
		},
		{
			name: "invalid parallel workers",
			mutate: func(cfg *Config) {
				cfg.AWS.ParallelWorkers = 0
			},
			wantErr: errParallelWorkersBounds,
		},
		{
			name: "invalid timeout",
			mutate: func(cfg *Config) {
				cfg.AWS.Timeout = 0
			},
			wantErr: errTimeoutInvalid,
		},
		{
			name: "empty naming pattern",
			mutate: func(cfg *Config) {
				cfg.NamingPattern = "  "
			},
			wantErr: errNamingPatternEmpty,
		},
		{
			name: "empty aws config file",
			mutate: func(cfg *Config) {
				cfg.AWS.ConfigFile = ""
			},
			wantErr: errAWSConfigFileEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := *base
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestExpandHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name     string
		path     string
		expected string
		wantErr  bool
	}{
		{
			name:     "empty path",
			path:     "",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "absolute path",
			path:     "/absolute/path",
			expected: "/absolute/path",
			wantErr:  false,
		},
		{
			name:     "tilde only",
			path:     "~",
			expected: home,
			wantErr:  false,
		},
		{
			name:     "tilde with path",
			path:     "~/subdir/file",
			expected: filepath.Join(home, "subdir/file"),
			wantErr:  false,
		},
		{
			name:     "tilde not at start",
			path:     "/some/~path",
			expected: "/some/~path",
			wantErr:  false,
		},
		{
			name:     "tilde with username style",
			path:     "~user/path",
			expected: "~user/path",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandHomeDir(tt.path)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expandHomeDir(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestExpandHomeDirNoHome(t *testing.T) {
	t.Setenv("HOME", "")

	_, err := expandHomeDir("~")
	if err == nil {
		t.Error("expected error when HOME is empty")
	}

	_, err = expandHomeDir("~/path")
	if err == nil {
		t.Error("expected error when HOME is empty")
	}
}

func TestNormalizePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name     string
		path     string
		wantErr  bool
		expected string
	}{
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			path:    "   ",
			wantErr: true,
		},
		{
			name:    "relative path",
			path:    "relative/path",
			wantErr: true,
		},
		{
			name:     "absolute path",
			path:     "/absolute/path",
			expected: "/absolute/path",
			wantErr:  false,
		},
		{
			name:     "tilde path",
			path:     "~/config",
			expected: filepath.Join(home, "config"),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizePath(tt.path, errConfigPathEmpty)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestConfigFromMapWithPartialOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	raw := map[string]interface{}{
		"config_path": "/custom/path",
	}

	cfg, err := ConfigFromMap(raw)
	if err != nil {
		t.Fatalf("ConfigFromMap failed: %v", err)
	}

	if cfg.ConfigPath != "/custom/path" {
		t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, "/custom/path")
	}

	if cfg.AWS.ConfigFile != filepath.Join(home, ".aws", "config") {
		t.Errorf("ConfigFile should use default, got %q", cfg.AWS.ConfigFile)
	}

	if cfg.AWS.CredentialsFile != filepath.Join(home, ".aws", "credentials") {
		t.Errorf("CredentialsFile should use default")
	}

	if cfg.NamingPattern != defaultNamingPattern() {
		t.Errorf("NamingPattern should use default")
	}

	if len(cfg.ManualConfigs) != 0 {
		t.Errorf("ManualConfigs = %v", cfg.ManualConfigs)
	}
}

func TestConfigValidateWithMergeOnlySkipsAWS(t *testing.T) {
	baseDir := t.TempDir()

	cfg := &Config{
		Enabled:    true,
		MergeOnly:  true,
		ConfigPath: filepath.Join(baseDir, "kube", "config"),
		AWS: AWSConfig{
			CredentialsFile: "",
			Regions:         nil,
			ParallelWorkers: 0,
			Timeout:         0,
		},
		NamingPattern: defaultNamingPattern(),
		Merge: MergeConfig{
			SourceDir: filepath.Join(baseDir, "kube"),
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected no error for merge-only mode, got: %v", err)
	}
}

func TestConfigFromMapWithMergeSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	raw := map[string]interface{}{
		"merge_enabled": true,
		"merge_only":    true,
		"merge": map[string]interface{}{
			"source_dir":       "/custom/source",
			"include_patterns": []string{"*.conf"},
			"exclude_patterns": []string{"*.tmp"},
		},
	}

	cfg, err := ConfigFromMap(raw)
	if err != nil {
		t.Fatalf("ConfigFromMap failed: %v", err)
	}

	if !cfg.MergeEnabled {
		t.Error("expected MergeEnabled to be true")
	}
	if !cfg.MergeOnly {
		t.Error("expected MergeOnly to be true")
	}
	if cfg.Merge.SourceDir != "/custom/source" {
		t.Errorf("Merge.SourceDir = %q", cfg.Merge.SourceDir)
	}
	if !reflect.DeepEqual(cfg.Merge.IncludePatterns, []string{"*.conf"}) {
		t.Errorf("IncludePatterns = %v", cfg.Merge.IncludePatterns)
	}
	if !reflect.DeepEqual(cfg.Merge.ExcludePatterns, []string{"*.tmp"}) {
		t.Errorf("ExcludePatterns = %v", cfg.Merge.ExcludePatterns)
	}
}

func TestConfigFromMapWithDisabledProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	raw := map[string]interface{}{
		"enabled": false,
	}

	cfg, err := ConfigFromMap(raw)
	if err != nil {
		t.Fatalf("ConfigFromMap failed: %v", err)
	}

	if cfg.Enabled {
		t.Error("expected Enabled to be false")
	}
}

func TestConfigFromMapWithAWSRoles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	raw := map[string]interface{}{
		"aws": map[string]interface{}{
			"roles": []string{"AdminAccess", "ReadOnly"},
		},
	}

	cfg, err := ConfigFromMap(raw)
	if err != nil {
		t.Fatalf("ConfigFromMap failed: %v", err)
	}

	if !reflect.DeepEqual(cfg.AWS.Roles, []string{"AdminAccess", "ReadOnly"}) {
		t.Errorf("Roles = %v", cfg.AWS.Roles)
	}
}

func TestConfigIsEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: false,
		},
		{
			name: "enabled config",
			cfg:  &Config{Enabled: true},
			want: true,
		},
		{
			name: "disabled config",
			cfg:  &Config{Enabled: false},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigValidateWithMergeOnly(t *testing.T) {
	baseDir := t.TempDir()

	cfg := &Config{
		Enabled:    true,
		MergeOnly:  true,
		ConfigPath: filepath.Join(baseDir, "kube", "config"),
		AWS: AWSConfig{
			CredentialsFile: "",
			Regions:         nil,
			ParallelWorkers: 0,
			Timeout:         0,
		},
		NamingPattern: defaultNamingPattern(),
		Merge: MergeConfig{
			SourceDir: filepath.Join(baseDir, "kube"),
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected no error for merge-only mode, got: %v", err)
	}

	if !cfg.MergeEnabled {
		t.Error("MergeEnabled should be set when MergeOnly is true")
	}
}
