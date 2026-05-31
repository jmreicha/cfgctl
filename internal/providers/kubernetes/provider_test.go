package kubernetes

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmreicha/cfgctl/internal/core"
)

func TestProvider_Name(t *testing.T) {
	provider := NewProvider(nil)

	if got := provider.Name(); got != ProviderName {
		t.Errorf("Name() = %q, want %q", got, ProviderName)
	}
}

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "nil config",
			config: nil,
		},
		{
			name: "with config",
			config: &Config{
				ConfigPath: "/home/user/.kube/config",
				AWS: AWSConfig{
					Regions:         []string{"us-east-1"},
					ParallelWorkers: 5,
				},
				NamingPattern: "{{.Profile}}-{{.Region}}-{{.Name}}",
				Enabled:       true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(tt.config)
			if provider == nil {
				t.Fatal("NewProvider() returned nil")
			}

			if provider.config == nil {
				t.Error("provider.config is nil")
			}
		})
	}
}

func TestProvider_Validate(t *testing.T) {
	tests := []struct {
		name        string
		provider    *Provider
		expectError bool
	}{
		{
			name: "valid provider with merge-only mode",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.MergeOnly = true
				return NewProvider(cfg)
			}(),
			expectError: false,
		},
		{
			name: "nil config",
			provider: &Provider{
				config: nil,
			},
			expectError: true,
		},
		{
			name: "invalid config - empty path",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = ""
				cfg.MergeOnly = true
				return NewProvider(cfg)
			}(),
			expectError: true,
		},
		{
			name: "disabled provider - skips validation",
			provider: NewProvider(&Config{
				ConfigPath: "",
				Enabled:    false,
			}),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := tt.provider.Validate(ctx)

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestProvider_Generate(t *testing.T) {
	tests := []struct {
		name        string
		provider    *Provider
		opts        *core.GenerateOptions
		expectError bool
	}{
		{
			name: "merge-only mode",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = filepath.Join(t.TempDir(), "config")
				cfg.MergeOnly = true
				return NewProvider(cfg)
			}(),
			opts: &core.GenerateOptions{
				Force:  true,
				DryRun: false,
			},
			expectError: false,
		},
		{
			name: "dry-run mode",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = filepath.Join(t.TempDir(), "config")
				cfg.MergeOnly = true
				return NewProvider(cfg)
			}(),
			opts: &core.GenerateOptions{
				Force:  false,
				DryRun: true,
			},
			expectError: false,
		},
		{
			name: "disabled provider",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = filepath.Join(t.TempDir(), "config")
				cfg.Enabled = false
				return NewProvider(cfg)
			}(),
			opts: &core.GenerateOptions{
				Force:  false,
				DryRun: false,
			},
			expectError: false,
		},
		{
			name: "merge only mode",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = filepath.Join(t.TempDir(), "config")
				cfg.MergeOnly = true
				return NewProvider(cfg)
			}(),
			opts: &core.GenerateOptions{
				Force:  true,
				DryRun: false,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tt.provider.Generate(ctx, tt.opts)

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result == nil && !tt.expectError {
				t.Error("expected result, got nil")
			}
		})
	}
}

func TestProvider_GenerateInvalidConfig(t *testing.T) {
	tests := []struct {
		name     string
		provider *Provider
		opts     *core.GenerateOptions
	}{
		{
			name: "empty config path",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = ""
				cfg.MergeOnly = true
				return NewProvider(cfg)
			}(),
			opts: &core.GenerateOptions{
				Force:  true,
				DryRun: false,
			},
		},
		{
			name: "nil config",
			provider: &Provider{
				config: nil,
			},
			opts: &core.GenerateOptions{
				Force:  true,
				DryRun: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := tt.provider.Generate(ctx, tt.opts)

			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestProvider_Backup(t *testing.T) {
	provider := NewProvider(&Config{
		ConfigPath: "/home/user/.kube/config",
		Enabled:    true,
	})

	ctx := context.Background()
	backupPath, err := provider.Backup(ctx)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if backupPath != "" {
		t.Errorf("expected empty backup path, got %q", backupPath)
	}
}

func TestProvider_Restore(t *testing.T) {
	provider := NewProvider(&Config{
		ConfigPath: "/home/user/.kube/config",
		Enabled:    true,
	})

	ctx := context.Background()
	err := provider.Restore(ctx, "/path/to/backup")

	if err == nil {
		t.Error("expected error for unimplemented restore, got nil")
	}
}

func TestProvider_Clean(t *testing.T) {
	t.Run("clean existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config")

		// Create a file to clean
		if err := os.WriteFile(configPath, []byte("test"), 0600); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		provider := NewProvider(&Config{
			ConfigPath: configPath,
			Enabled:    true,
		})

		ctx := context.Background()
		err := provider.Clean(ctx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Error("expected file to be removed")
		}
	})

	t.Run("clean non-existent file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config")

		provider := NewProvider(&Config{
			ConfigPath: configPath,
			Enabled:    true,
		})

		ctx := context.Background()
		err := provider.Clean(ctx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("nil config", func(t *testing.T) {
		provider := &Provider{
			config: nil,
		}

		ctx := context.Background()
		err := provider.Clean(ctx)

		if err != nil {
			t.Errorf("expected nil for nil config (no-op), got: %v", err)
		}
	})
}

func TestProvider_InterfaceCompliance(_ *testing.T) {
	var _ core.Provider = (*Provider)(nil)
}

func TestProvider_ValidateInvalidPaths(t *testing.T) {
	tests := []struct {
		name        string
		configPath  string
		expectError bool
	}{
		{
			name:        "empty path",
			configPath:  "",
			expectError: true,
		},
		{
			name:        "relative path",
			configPath:  "relative/path",
			expectError: true,
		},
		{
			name:        "absolute path with merge-only mode",
			configPath:  "/absolute/path",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.ConfigPath = tt.configPath
			cfg.MergeOnly = true
			provider := NewProvider(cfg)

			ctx := context.Background()
			err := provider.Validate(ctx)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestProvider_GenerateWithOptsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.ConfigPath = filepath.Join(tmpDir, "config")
	cfg.MergeOnly = true

	provider := NewProvider(cfg)

	// Create new config for opts
	optsConfig := DefaultConfig()
	optsConfig.ConfigPath = filepath.Join(tmpDir, "opts-config")
	optsConfig.MergeOnly = true

	opts := &core.GenerateOptions{
		Force:  true,
		DryRun: false,
		Config: optsConfig,
	}

	ctx := context.Background()
	result, err := provider.Generate(ctx, opts)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result == nil {
		t.Error("expected result, got nil")
	}

	// Verify the config was updated from opts
	if provider.config.ConfigPath != optsConfig.ConfigPath {
		t.Errorf("expected config path %q, got %q", optsConfig.ConfigPath, provider.config.ConfigPath)
	}
}

func TestProvider_GenerateExistingFileNoForce(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	// Create existing file
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("apiVersion: v1\nkind: Config\n"), 0600); err != nil {
		t.Fatalf("failed to create existing file: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = configPath
	cfg.MergeOnly = true

	provider := NewProvider(cfg)

	opts := &core.GenerateOptions{
		Force:  false,
		DryRun: false,
	}

	ctx := context.Background()
	result, err := provider.Generate(ctx, opts)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if len(result.FilesSkipped) != 1 {
		t.Errorf("expected 1 skipped file, got %d", len(result.FilesSkipped))
	}

	if len(result.Warnings) == 0 {
		t.Error("expected warning about existing file")
	}
}

func TestProvider_GenerateExistingFileSkipsDiscovery(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	if err := os.WriteFile(configPath, []byte("existing"), 0600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = configPath
	cfg.Enabled = true
	cfg.MergeOnly = false
	cfg.AWS.Regions = []string{"us-west-2"}

	provider := NewProvider(cfg, WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))

	opts := &core.GenerateOptions{Force: false, DryRun: false}
	result, err := provider.Generate(context.Background(), opts)
	if err != nil {
		t.Fatalf("expected no error (early exit), got: %v", err)
	}
	if len(result.FilesSkipped) != 1 {
		t.Errorf("expected 1 skipped file, got %d", len(result.FilesSkipped))
	}
	if len(result.FilesCreated) != 0 {
		t.Errorf("expected 0 created files, got %d", len(result.FilesCreated))
	}
}

func TestProvider_GenerateExistingFileForceProceeds(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	if err := os.WriteFile(configPath, []byte("apiVersion: v1\nkind: Config\n"), 0600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = configPath
	cfg.Enabled = true
	cfg.MergeOnly = true

	provider := NewProvider(cfg, WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))

	opts := &core.GenerateOptions{Force: true, DryRun: false}
	result, err := provider.Generate(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.FilesSkipped) != 0 {
		t.Errorf("expected 0 skipped files with --force, got %d", len(result.FilesSkipped))
	}
}

func TestProvider_GenerateDryRunBypassesExistingCheck(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	if err := os.WriteFile(configPath, []byte("apiVersion: v1\nkind: Config\n"), 0600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = configPath
	cfg.Enabled = true
	cfg.MergeOnly = true

	provider := NewProvider(cfg, WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))

	opts := &core.GenerateOptions{Force: false, DryRun: true}
	result, err := provider.Generate(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.FilesSkipped) != 0 {
		t.Errorf("dry-run should bypass existing check, got %d skipped", len(result.FilesSkipped))
	}
}

// wrongConfig is a ProviderConfig that is not *Config, used for type mismatch tests.
type wrongConfig struct{}

func (w *wrongConfig) Validate() error { return nil }
func (w *wrongConfig) IsEnabled() bool { return true }

const testConfigPath = "/tmp/test-config"

func TestProvider_NeedsBackup(t *testing.T) {
	tests := []struct {
		name       string
		provider   *Provider
		opts       *core.GenerateOptions
		setupFile  bool
		wantBackup bool
		wantErr    bool
	}{
		{
			name:       "nil config returns false",
			provider:   &Provider{config: nil},
			opts:       nil,
			wantBackup: false,
		},
		{
			name: "dry-run returns false",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = testConfigPath
				return NewProvider(cfg)
			}(),
			opts:       &core.GenerateOptions{DryRun: true},
			wantBackup: false,
		},
		{
			name: "disabled provider returns false",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.Enabled = false
				cfg.ConfigPath = testConfigPath
				return NewProvider(cfg)
			}(),
			opts:       &core.GenerateOptions{Force: true},
			wantBackup: false,
		},
		{
			name: "file exists without force returns false",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = "" // set in test body
				return NewProvider(cfg)
			}(),
			opts:       &core.GenerateOptions{Force: false},
			setupFile:  true,
			wantBackup: false,
		},
		{
			name: "file exists with force returns true",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = "" // set in test body
				return NewProvider(cfg)
			}(),
			opts:       &core.GenerateOptions{Force: true},
			setupFile:  true,
			wantBackup: true,
		},
		{
			name: "file does not exist returns false",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = "/nonexistent/path/that/does/not/exist"
				return NewProvider(cfg)
			}(),
			opts:       &core.GenerateOptions{Force: false},
			wantBackup: false,
		},
		{
			name: "nil opts returns false when file exists",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = "" // set in test body
				return NewProvider(cfg)
			}(),
			opts:       nil,
			setupFile:  true,
			wantBackup: false,
		},
		{
			name: "nil opts returns false when file does not exist",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = "/nonexistent/path/config"
				return NewProvider(cfg)
			}(),
			opts:       nil,
			wantBackup: false,
		},
		{
			name: "opts with config field set is ignored by NeedsBackup",
			provider: func() *Provider {
				cfg := DefaultConfig()
				cfg.ConfigPath = testConfigPath
				return NewProvider(cfg)
			}(),
			opts:       &core.GenerateOptions{Config: &wrongConfig{}},
			wantBackup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFile && tt.provider.config != nil {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "config")
				if err := os.WriteFile(configPath, []byte("test"), 0600); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				tt.provider.config.ConfigPath = configPath
			}

			got, err := tt.provider.NeedsBackup(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantBackup {
				t.Errorf("NeedsBackup() = %v, want %v", got, tt.wantBackup)
			}
		})
	}
}

func TestWithLogger(t *testing.T) {
	t.Run("nil logger keeps default", func(t *testing.T) {
		provider := NewProvider(nil, WithLogger(nil))
		if provider.logger == nil {
			t.Error("expected non-nil logger when WithLogger(nil) is used")
		}
	})

	t.Run("non-nil logger is set", func(t *testing.T) {
		customLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		provider := NewProvider(nil, WithLogger(customLogger))
		if provider.logger != customLogger {
			t.Error("expected custom logger to be set")
		}
	})
}

func TestProvider_CleanEmptyConfigPath(t *testing.T) {
	provider := NewProvider(&Config{
		ConfigPath: "",
		Enabled:    true,
	})

	ctx := context.Background()
	err := provider.Clean(ctx)
	if err != nil {
		t.Errorf("expected nil error for empty config path, got: %v", err)
	}
}

func TestProvider_writeKubeconfig(t *testing.T) {
	t.Run("empty path returns error", func(t *testing.T) {
		provider := NewProvider(nil)
		result := &core.Result{
			FilesCreated: []string{},
			FilesSkipped: []string{},
			Warnings:     []string{},
		}
		err := provider.writeKubeconfig("", newKubeconfig(), result)
		if err == nil {
			t.Fatal("expected error for empty path, got nil")
		}
	})

	t.Run("file exists overwrites", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config")
		if err := os.WriteFile(configPath, []byte("test"), 0600); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		provider := NewProvider(nil)
		result := &core.Result{
			FilesCreated: []string{},
			FilesSkipped: []string{},
			Warnings:     []string{},
		}
		err := provider.writeKubeconfig(configPath, newKubeconfig(), result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.FilesCreated) != 1 {
			t.Errorf("expected 1 created file, got %d", len(result.FilesCreated))
		}
	})

	t.Run("new file is created", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "subdir", "config")

		provider := NewProvider(nil)
		result := &core.Result{
			FilesCreated: []string{},
			FilesSkipped: []string{},
			Warnings:     []string{},
		}
		err := provider.writeKubeconfig(configPath, newKubeconfig(), result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.FilesCreated) != 1 {
			t.Errorf("expected 1 created file, got %d", len(result.FilesCreated))
		}
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("expected file to be created")
		}
	})
}

func TestProvider_discoverClusters(t *testing.T) {
	t.Run("merge-only mode returns nil", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MergeOnly = true
		provider := NewProvider(cfg)

		clusters, warnings, err := provider.discoverClusters(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clusters != nil {
			t.Errorf("expected nil clusters, got %v", clusters)
		}
		if warnings != nil {
			t.Errorf("expected nil warnings, got %v", warnings)
		}
	})
}

func TestProvider_buildKubeconfig(t *testing.T) {
	t.Run("merge-only mode with no discovered clusters", func(t *testing.T) {
		tmpDir := t.TempDir()
		mergeDir := filepath.Join(tmpDir, "merge")
		if err := os.MkdirAll(mergeDir, 0700); err != nil {
			t.Fatalf("failed to create merge dir: %v", err)
		}

		cfg := DefaultConfig()
		cfg.ConfigPath = filepath.Join(tmpDir, "config")
		cfg.MergeOnly = true
		cfg.MergeEnabled = true
		cfg.Merge = MergeConfig{
			SourceDir:       mergeDir,
			IncludePatterns: []string{"*.yaml"},
			ExcludePatterns: []string{"*.bak"},
		}

		provider := NewProvider(cfg)

		mergeConfig, _, err := provider.buildKubeconfig(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mergeConfig == nil {
			t.Fatal("expected non-nil merged config")
		}
	})

	t.Run("merge disabled with discovered clusters", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MergeEnabled = false
		cfg.MergeOnly = false

		provider := NewProvider(cfg)

		clusters := []DiscoveredCluster{
			{
				Profile:  "prod",
				Region:   "us-west-2",
				Name:     "test-cluster",
				Endpoint: "https://test.example.com",
				CAData:   []byte("ca-data"),
			},
		}

		result, files, err := provider.buildKubeconfig(clusters, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil config")
			return
		}
		if len(files) != 0 {
			t.Errorf("expected no merge files, got %d", len(files))
		}
		if _, ok := result.Clusters["prod-test-cluster"]; !ok {
			t.Error("expected cluster to be in config")
		}
	})

	t.Run("no clusters and merge disabled returns nil", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MergeEnabled = false
		cfg.MergeOnly = false

		provider := NewProvider(cfg)

		result, files, err := provider.buildKubeconfig(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Error("expected nil config when no clusters and merge disabled")
		}
		if files != nil {
			t.Error("expected nil files")
		}
	})

	t.Run("merge enabled with discovered clusters", func(t *testing.T) {
		tmpDir := t.TempDir()
		mergeDir := filepath.Join(tmpDir, "merge")
		if err := os.MkdirAll(mergeDir, 0700); err != nil {
			t.Fatalf("failed to create merge dir: %v", err)
		}

		cfg := DefaultConfig()
		cfg.ConfigPath = filepath.Join(tmpDir, "config")
		cfg.MergeEnabled = true
		cfg.MergeOnly = false
		cfg.Merge = MergeConfig{
			SourceDir:       mergeDir,
			IncludePatterns: []string{"*.yaml"},
			ExcludePatterns: []string{"*.bak"},
		}

		provider := NewProvider(cfg)

		clusters := []DiscoveredCluster{
			{
				Profile:  "prod",
				Region:   "us-west-2",
				Name:     "test-cluster",
				Endpoint: "https://test.example.com",
				CAData:   []byte("ca-data"),
			},
		}

		result, _, err := provider.buildKubeconfig(clusters, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil config")
			return
		}
		if _, ok := result.Clusters["prod-test-cluster"]; !ok {
			t.Error("expected cluster to be in merged config")
		}
	})

	t.Run("replace drops unmanaged existing entries", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config")

		existingKubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://manual.example.com
  name: manual-cluster
contexts:
- context:
    cluster: manual-cluster
    user: manual-user
  name: manual-context
users:
- name: manual-user
  user: {}
current-context: manual-context
`
		if err := os.WriteFile(configPath, []byte(existingKubeconfig), 0600); err != nil {
			t.Fatalf("write existing kubeconfig: %v", err)
		}

		cfg := DefaultConfig()
		cfg.ConfigPath = configPath
		cfg.MergeEnabled = false
		cfg.MergeOnly = false

		provider := NewProvider(cfg)

		clusters := []DiscoveredCluster{
			{
				Profile:  "prod",
				Region:   "us-east-1",
				Name:     "managed-cluster",
				Endpoint: "https://managed.example.com",
				CAData:   []byte("ca-data"),
			},
		}

		result, _, err := provider.buildKubeconfig(clusters, &core.GenerateOptions{Replace: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil config")
			return
		}
		if _, ok := result.Clusters["manual-cluster"]; ok {
			t.Error("manual-cluster should be dropped with --replace")
		}
		if _, ok := result.Clusters["prod-managed-cluster"]; !ok {
			t.Error("managed cluster should be present")
		}
	})
}

func TestProvider_buildKubeconfigManualConfigs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MergeEnabled = false
	cfg.MergeOnly = false
	cfg.ManualConfigs = []ManualConfig{
		{
			Name:            "manual-cluster",
			ClusterEndpoint: "https://manual.example.com",
			AuthInfo: ManualAuthInfo{
				Token: "manual-token",
			},
			ContextSettings: ManualContext{
				Namespace: "manual",
			},
		},
	}

	provider := NewProvider(cfg)

	result, files, err := provider.buildKubeconfig(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil config")
	}
	if files != nil {
		t.Errorf("expected nil files, got %v", files)
	}

	cluster := result.Clusters["manual-cluster"]
	if cluster == nil {
		t.Fatal("expected manual cluster to be in config")
	}
	if cluster.Server != "https://manual.example.com" {
		t.Errorf("manual cluster.Server = %q", cluster.Server)
	}

	context := result.Contexts["manual-cluster"]
	if context == nil {
		t.Fatal("expected manual context to be in config")
	}
	if context.Namespace != "manual" {
		t.Errorf("manual context.Namespace = %q", context.Namespace)
	}
}

func TestProvider_buildKubeconfigManualOverridesDiscovered(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MergeEnabled = false
	cfg.MergeOnly = false
	cfg.ManualConfigs = []ManualConfig{
		{
			Name:            "prod-test-cluster",
			ClusterEndpoint: "https://manual.example.com",
			ContextSettings: ManualContext{
				Namespace: "manual",
			},
		},
	}

	provider := NewProvider(cfg)

	clusters := []DiscoveredCluster{
		{
			Profile:  "prod",
			Region:   "us-west-2",
			Name:     "test-cluster",
			Endpoint: "https://test.example.com",
			CAData:   []byte("ca-data"),
		},
	}

	result, files, err := provider.buildKubeconfig(clusters, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil config")
	}
	if files != nil {
		t.Errorf("expected nil files, got %v", files)
	}

	cluster := result.Clusters["prod-test-cluster"]
	if cluster == nil {
		t.Fatal("expected cluster to be in config")
	}
	if cluster.Server != "https://manual.example.com" {
		t.Errorf("cluster.Server = %q", cluster.Server)
	}

	context := result.Contexts["prod-test-cluster"]
	if context == nil {
		t.Fatal("expected context to be in config")
	}
	if context.Namespace != "manual" {
		t.Errorf("context.Namespace = %q", context.Namespace)
	}
}

func TestProvider_validateAndPrepare(t *testing.T) {
	t.Run("wrong config type in opts returns error", func(t *testing.T) {
		provider := NewProvider(DefaultConfig())
		opts := &core.GenerateOptions{
			Config: &wrongConfig{},
		}
		err := provider.validateAndPrepare(opts)
		if err == nil {
			t.Fatal("expected error for wrong config type, got nil")
		}
	})
}
