package granted

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
				ConfigPath:     "/home/user/.granted/config",
				DefaultBrowser: "FIREFOX",
				Enabled:        true,
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
			name: "valid provider",
			provider: NewProvider(&Config{
				ConfigPath: "/home/user/.granted/config",
				Enabled:    true,
			}),
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
			provider: NewProvider(&Config{
				ConfigPath: "",
				Enabled:    true,
			}),
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
	tmpDir := t.TempDir()

	provider := NewProvider(&Config{
		ConfigPath:                 filepath.Join(tmpDir, ".granted", "config"),
		CredentialProcessAutoLogin: true,
		DefaultBrowser:             "STDOUT",
		DisableUsageTips:           true,
		Enabled:                    true,
	})

	tests := []struct {
		name string
		opts *core.GenerateOptions
	}{
		{
			name: "normal mode",
			opts: &core.GenerateOptions{
				DryRun: false,
				Force:  false,
			},
		},
		{
			name: "dry-run mode",
			opts: &core.GenerateOptions{
				DryRun: true,
			},
		},
		{
			name: "force mode",
			opts: &core.GenerateOptions{
				Force: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := provider.Generate(ctx, tt.opts)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("result is nil")
			}

			if result.Provider != ProviderName {
				t.Errorf("Provider = %q, want %q", result.Provider, ProviderName)
			}

			if result.FilesCreated == nil {
				t.Error("FilesCreated is nil")
			}

			if result.FilesSkipped == nil {
				t.Error("FilesSkipped is nil")
			}

			if tt.opts.DryRun && len(result.Warnings) == 0 {
				t.Error("expected warnings in dry-run mode")
			}
		})
	}
}

func TestProvider_GenerateIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".granted", "config")

	provider := NewProvider(&Config{
		ConfigPath:                 configPath,
		CredentialProcessAutoLogin: true,
		CustomBrowserPath:          "/usr/bin/firefox",
		CustomSSOBrowserPath:       "/usr/bin/chrome",
		DefaultBrowser:             "FIREFOX",
		DisableUsageTips:           false,
		Enabled:                    true,
		ExportCredentialSuffix:     "_CUSTOM",
		Ordering:                   "alphabetical",
	})

	ctx := context.Background()

	err := provider.Validate(ctx)
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	opts := &core.GenerateOptions{
		DryRun: false,
		Force:  false,
	}

	result, err := provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	if len(result.FilesCreated) != 1 || result.FilesCreated[0] != configPath {
		t.Errorf("expected config file to be created at %s, got %v", configPath, result.FilesCreated)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("config file not found: %v", err)
	}

	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	expectedContent := `DefaultBrowser = "FIREFOX"
CustomBrowserPath = "/usr/bin/firefox"
CustomSSOBrowserPath = "/usr/bin/chrome"
Ordering = "alphabetical"
ExportCredentialSuffix = "_CUSTOM"
DisableUsageTips = false
CredentialProcessAutoLogin = true
`
	if string(content) != expectedContent {
		t.Errorf("content mismatch:\ngot:\n%s\nwant:\n%s", content, expectedContent)
	}
}

func TestProvider_GenerateCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "path", ".granted", "config")

	provider := NewProvider(&Config{
		ConfigPath:     nestedPath,
		DefaultBrowser: "STDOUT",
		Enabled:        true,
	})

	ctx := context.Background()
	opts := &core.GenerateOptions{
		DryRun: false,
		Force:  false,
	}

	result, err := provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	if len(result.FilesCreated) != 1 {
		t.Errorf("expected 1 file created, got %d", len(result.FilesCreated))
	}

	if _, err := os.Stat(nestedPath); err != nil {
		t.Errorf("config file not created: %v", err)
	}

	dirPath := filepath.Dir(nestedPath)
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}

	if info.Mode().Perm() != 0700 {
		t.Errorf("directory permissions = %o, want 0700", info.Mode().Perm())
	}
}

func TestProvider_GenerateWithExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	existingContent := `DefaultBrowser = "CHROME"
`
	if err := os.WriteFile(configPath, []byte(existingContent), 0600); err != nil {
		t.Fatalf("failed to create existing config: %v", err)
	}

	provider := NewProvider(&Config{
		ConfigPath:     configPath,
		DefaultBrowser: "STDOUT",
		Enabled:        true,
	})

	ctx := context.Background()

	opts := &core.GenerateOptions{
		DryRun: false,
		Force:  false,
	}

	result, err := provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	if len(result.FilesSkipped) != 1 {
		t.Error("expected file to be skipped without force")
	}

	content, _ := os.ReadFile(configPath)
	if string(content) != existingContent {
		t.Error("existing config was modified without force")
	}

	opts.Force = true

	result, err = provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("generation with force failed: %v", err)
	}

	if len(result.FilesCreated) != 1 {
		t.Error("expected file to be created with force")
	}

	content, _ = os.ReadFile(configPath)
	if !strings.Contains(string(content), `DefaultBrowser = "STDOUT"`) {
		t.Error("config was not overwritten with force")
	}
}

func TestProvider_GenerateDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".granted", "config")

	provider := NewProvider(&Config{
		ConfigPath:     configPath,
		DefaultBrowser: "STDOUT",
		Enabled:        true,
	})

	ctx := context.Background()

	opts := &core.GenerateOptions{
		DryRun: true,
		Force:  false,
	}

	result, err := provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("dry-run generation failed: %v", err)
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("config file should not exist in dry-run mode")
	}

	if len(result.Warnings) == 0 {
		t.Error("expected warnings in dry-run mode")
	}

	foundDryRunWarning := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "dry-run") {
			foundDryRunWarning = true
			break
		}
	}
	if !foundDryRunWarning {
		t.Error("expected dry-run warning in results")
	}

	if _, ok := result.Metadata["config_path"]; !ok {
		t.Error("expected config_path in metadata")
	}

	if _, ok := result.Metadata["config_content"]; !ok {
		t.Error("expected config_content in metadata")
	}
}

func TestProvider_GenerateDisabled(t *testing.T) {
	provider := NewProvider(&Config{
		ConfigPath: "/tmp/config",
		Enabled:    false,
	})

	ctx := context.Background()

	opts := &core.GenerateOptions{
		DryRun: false,
		Force:  false,
	}

	result, err := provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	if len(result.FilesCreated) != 0 {
		t.Error("expected no files to be created when provider is disabled")
	}

	foundDisabledWarning := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "disabled") {
			foundDisabledWarning = true
			break
		}
	}
	if !foundDisabledWarning {
		t.Error("expected disabled warning in results")
	}
}

func TestProvider_GenerateInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "empty config path",
			config: &Config{
				ConfigPath: "",
				Enabled:    true,
			},
		},
		{
			name: "relative config path",
			config: &Config{
				ConfigPath: "relative/path",
				Enabled:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(tt.config)
			ctx := context.Background()

			opts := &core.GenerateOptions{
				DryRun: false,
				Force:  false,
			}

			_, err := provider.Generate(ctx, opts)
			if err == nil {
				t.Error("expected error for invalid config")
			}
		})
	}
}

func TestProvider_GenerateNilConfig(t *testing.T) {
	provider := &Provider{config: nil}
	ctx := context.Background()

	opts := &core.GenerateOptions{
		DryRun: false,
		Force:  false,
	}

	_, err := provider.Generate(ctx, opts)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestProvider_GenerateWithOptsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	provider := NewProvider(&Config{
		ConfigPath:     configPath,
		DefaultBrowser: "STDOUT",
		Enabled:        true,
	})

	optsConfig := &Config{
		ConfigPath:     configPath,
		DefaultBrowser: "FIREFOX",
		Enabled:        true,
	}

	ctx := context.Background()
	opts := &core.GenerateOptions{
		DryRun: false,
		Force:  false,
		Config: optsConfig,
	}

	_, err := provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	content, _ := os.ReadFile(configPath)
	if !strings.Contains(string(content), `DefaultBrowser = "FIREFOX"`) {
		t.Error("opts.Config was not used")
	}
}

func TestProvider_GenerateWithInvalidOptsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	provider := NewProvider(&Config{
		ConfigPath:     configPath,
		DefaultBrowser: "STDOUT",
		Enabled:        true,
	})

	ctx := context.Background()
	opts := &core.GenerateOptions{
		DryRun: false,
		Force:  false,
		Config: &invalidConfig{},
	}

	_, err := provider.Generate(ctx, opts)
	if err == nil {
		t.Error("expected error for invalid opts.Config type")
	}
}

type invalidConfig struct{}

func (c *invalidConfig) Validate() error { return nil }

func TestProvider_Backup(t *testing.T) {
	provider := NewProvider(&Config{
		ConfigPath: "/home/user/.granted/config",
		Enabled:    true,
	})

	ctx := context.Background()
	backupPath, err := provider.Backup(ctx)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if backupPath != "" {
		t.Errorf("backupPath = %q, want empty string", backupPath)
	}
}

func TestProviderNeedsBackup(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	provider := NewProvider(&Config{ConfigPath: configPath, Enabled: true})

	if needsBackup, err := provider.NeedsBackup(&core.GenerateOptions{DryRun: true}); err != nil {
		t.Fatalf("NeedsBackup error: %v", err)
	} else if needsBackup {
		t.Fatal("expected NeedsBackup to be false for dry-run")
	}

	if err := os.WriteFile(configPath, []byte("data"), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if needsBackup, err := provider.NeedsBackup(&core.GenerateOptions{Force: false}); err != nil {
		t.Fatalf("NeedsBackup error: %v", err)
	} else if needsBackup {
		t.Fatal("expected NeedsBackup to be false when config exists without force")
	}

	if needsBackup, err := provider.NeedsBackup(&core.GenerateOptions{Force: true}); err != nil {
		t.Fatalf("NeedsBackup error: %v", err)
	} else if !needsBackup {
		t.Fatal("expected NeedsBackup to be true when forcing overwrite")
	}
}

func TestProviderNeedsBackupDisabled(t *testing.T) {
	provider := NewProvider(&Config{ConfigPath: "/tmp/config", Enabled: false})

	if needsBackup, err := provider.NeedsBackup(&core.GenerateOptions{Force: true}); err != nil {
		t.Fatalf("NeedsBackup error: %v", err)
	} else if needsBackup {
		t.Fatal("expected NeedsBackup to be false when provider is disabled")
	}
}

func TestProvider_Restore(t *testing.T) {
	provider := NewProvider(&Config{
		ConfigPath: "/home/user/.granted/config",
		Enabled:    true,
	})

	tests := []struct {
		name        string
		backupPath  string
		expectError bool
	}{
		{
			name:        "empty backup path",
			backupPath:  "",
			expectError: true,
		},
		{
			name:        "non-empty backup path",
			backupPath:  "/tmp/backup",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := provider.Restore(ctx, tt.backupPath)

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestProvider_Clean(t *testing.T) {
	provider := NewProvider(&Config{
		ConfigPath: "/home/user/.granted/config",
		Enabled:    true,
	})

	ctx := context.Background()
	err := provider.Clean(ctx)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProvider_InterfaceCompliance(_ *testing.T) {
	var _ core.Provider = (*Provider)(nil)
}

func TestProvider_BuildConfigContent(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected string
	}{
		{
			name: "default values",
			config: &Config{
				CredentialProcessAutoLogin: true,
				CustomBrowserPath:          "",
				CustomSSOBrowserPath:       "",
				DefaultBrowser:             "STDOUT",
				DisableUsageTips:           true,
				ExportCredentialSuffix:     "",
				Ordering:                   "",
			},
			expected: `DefaultBrowser = "STDOUT"
CustomBrowserPath = ""
CustomSSOBrowserPath = ""
Ordering = ""
ExportCredentialSuffix = ""
DisableUsageTips = true
CredentialProcessAutoLogin = true
`,
		},
		{
			name: "custom values",
			config: &Config{
				CredentialProcessAutoLogin: false,
				CustomBrowserPath:          "/usr/bin/firefox",
				CustomSSOBrowserPath:       "/usr/bin/chrome",
				DefaultBrowser:             "FIREFOX",
				DisableUsageTips:           false,
				ExportCredentialSuffix:     "_SUFFIX",
				Ordering:                   "alphabetical",
			},
			expected: `DefaultBrowser = "FIREFOX"
CustomBrowserPath = "/usr/bin/firefox"
CustomSSOBrowserPath = "/usr/bin/chrome"
Ordering = "alphabetical"
ExportCredentialSuffix = "_SUFFIX"
DisableUsageTips = false
CredentialProcessAutoLogin = false
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &Provider{config: tt.config}
			content := provider.buildConfigContent()

			if content != tt.expected {
				t.Errorf("buildConfigContent() mismatch:\ngot:\n%s\nwant:\n%s", content, tt.expected)
			}
		})
	}
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
			name:        "absolute path",
			configPath:  "/tmp/granted/config",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(&Config{
				ConfigPath: tt.configPath,
				Enabled:    true,
			})

			ctx := context.Background()
			err := provider.Validate(ctx)

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestProvider_GenerateMkdirError(t *testing.T) {
	provider := NewProvider(&Config{
		ConfigPath:     "/root/nonexistent/directory/that/cannot/be/created/config",
		DefaultBrowser: "STDOUT",
		Enabled:        true,
	})

	ctx := context.Background()
	opts := &core.GenerateOptions{
		DryRun: false,
		Force:  false,
	}

	_, err := provider.Generate(ctx, opts)
	if err == nil {
		t.Error("expected error for mkdir failure")
	}
}

func TestProvider_GenerateWriteFileError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	if err := os.WriteFile(configPath, []byte("test"), 0000); err != nil {
		t.Fatalf("failed to create read-only file: %v", err)
	}

	provider := NewProvider(&Config{
		ConfigPath:     configPath,
		DefaultBrowser: "STDOUT",
		Enabled:        true,
	})

	ctx := context.Background()
	opts := &core.GenerateOptions{
		DryRun: false,
		Force:  true,
	}

	_, err := provider.Generate(ctx, opts)
	if err == nil {
		t.Error("expected error for write failure")
	}
}
