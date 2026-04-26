package aws

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmreicha/cfgctl/internal/core"
)

// wrongConfig is a ProviderConfig that is not *Config, used for type assertion tests.
type wrongConfig struct{}

func (w *wrongConfig) Validate() error { return nil }

func TestProviderName(t *testing.T) {
	provider := NewProvider(nil)
	if provider.Name() != ProviderName {
		t.Fatalf("name = %q", provider.Name())
	}
}

func TestProviderValidateDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	provider := NewProvider(cfg)

	if err := provider.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestProviderGenerateDisabled(t *testing.T) {
	cacheDir := t.TempDir()
	configDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.ConfigPath = filepath.Join(configDir, "config")
	cfg.Enabled = false
	cfg.SSO.Region = testRegion
	cfg.SSO.StartURL = testStartURL
	cfg.TokenCachePaths = []string{cacheDir}
	provider := NewProvider(cfg)

	result, err := provider.Generate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings")
	}
}

func TestProviderGenerateCredentialsDisabledWhenUseCredentialProcessFalse(t *testing.T) {
	configDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.ConfigPath = filepath.Join(configDir, "config")
	cfg.CredentialsPath = filepath.Join(configDir, "credentials")
	cfg.GenerateCredentials = true
	cfg.UseCredentialProcess = false
	cfg.SSO.Region = testRegion
	cfg.SSO.StartURL = testStartURL
	cfg.TokenCachePaths = []string{t.TempDir()}
	provider := NewProvider(cfg)
	provider.discover = func(context.Context, *Config) ([]DiscoveredProfile, error) {
		return []DiscoveredProfile{}, nil
	}

	result, err := provider.Generate(context.Background(), &core.GenerateOptions{Force: true})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if _, err := os.Stat(cfg.CredentialsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no credentials file, got %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warning about credentials generation")
	}
}

func TestProviderGenerateCredentialsWithCredentialProcess(t *testing.T) {
	configDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.ConfigPath = filepath.Join(configDir, "config")
	cfg.CredentialsPath = filepath.Join(configDir, "credentials")
	cfg.GenerateCredentials = true
	cfg.UseCredentialProcess = true
	cfg.SSO.Region = testRegion
	cfg.SSO.StartURL = testStartURL
	cfg.TokenCachePaths = []string{t.TempDir()}
	provider := NewProvider(cfg)
	provider.discover = func(context.Context, *Config) ([]DiscoveredProfile, error) {
		return []DiscoveredProfile{
			{
				AccountID:   "111111111111",
				AccountName: "prod",
				RoleName:    "Admin",
			},
		}, nil
	}

	result, err := provider.Generate(context.Background(), &core.GenerateOptions{Force: true})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if _, err := os.Stat(cfg.CredentialsPath); err != nil {
		t.Fatalf("expected credentials file: %v", err)
	}
}

func TestProviderBackupNoFile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfigPath = filepath.Join(t.TempDir(), "nonexistent")
	provider := NewProvider(cfg)

	backup, err := provider.Backup(context.Background())
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	if backup != "" {
		t.Fatalf("expected empty backup path, got %q", backup)
	}
}

func TestProviderBackupCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte("[default]\nregion = us-east-1\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = cfgPath
	provider := NewProvider(cfg)

	backup, err := provider.Backup(context.Background())
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	if !strings.HasPrefix(backup, cfgPath+".") || !strings.HasSuffix(backup, ".bak") {
		t.Fatalf("backup = %q, expected timestamped .bak file", backup)
	}

	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != "[default]\nregion = us-east-1\n" {
		t.Fatalf("backup content = %q", string(data))
	}
}

func TestProviderNeedsBackup(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config")
	provider := NewProvider(&Config{ConfigPath: configPath, Enabled: true})

	if needsBackup, err := provider.NeedsBackup(&core.GenerateOptions{DryRun: true}); err != nil {
		t.Fatalf("NeedsBackup error: %v", err)
	} else if needsBackup {
		t.Fatal("expected NeedsBackup to be false for dry-run")
	}

	if err := os.WriteFile(configPath, []byte("data"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
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

func TestProviderRestore(t *testing.T) {
	provider := NewProvider(DefaultConfig())

	if err := provider.Restore(context.Background(), "path"); err == nil {
		t.Fatal("expected restore error")
	}
}

func TestProviderCleanEmptyPath(t *testing.T) {
	provider := NewProvider(&Config{ConfigPath: ""})

	if err := provider.Clean(context.Background()); err != nil {
		t.Fatalf("Clean failed: %v", err)
	}
}

func TestProviderCleanRemovesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte("[default]\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	provider := NewProvider(&Config{ConfigPath: cfgPath})
	if err := provider.Clean(context.Background()); err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	if _, err := os.Stat(cfgPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected file to be removed, got err: %v", err)
	}
}

func TestProviderCleanNonexistentFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nonexistent")

	provider := NewProvider(&Config{ConfigPath: cfgPath})
	if err := provider.Clean(context.Background()); err != nil {
		t.Fatalf("Clean failed: %v", err)
	}
}

func TestProviderCleanNilConfig(t *testing.T) {
	provider := &Provider{config: nil}
	if err := provider.Clean(context.Background()); err != nil {
		t.Fatalf("expected nil for nil config (no-op), got: %v", err)
	}
}

func TestApplyDryRunMetadata(t *testing.T) {
	result := &core.Result{
		Provider:     ProviderName,
		FilesCreated: []string{},
		FilesSkipped: []string{},
		Warnings:     []string{},
		Metadata:     make(map[string]interface{}),
	}

	applyDryRunMetadata(result, "/tmp/config", "config-content", false, "", "")

	if result.Metadata["config_path"] != "/tmp/config" {
		t.Fatalf("config_path = %v", result.Metadata["config_path"])
	}
	if result.Metadata["config_content"] != "config-content" {
		t.Fatalf("config_content = %v", result.Metadata["config_content"])
	}
	if _, ok := result.Metadata["credentials_path"]; ok {
		t.Fatal("expected no credentials_path when credentials disabled")
	}

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "dry-run") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dry-run warning")
	}
}

func TestApplyDryRunMetadataWithCredentials(t *testing.T) {
	result := &core.Result{
		Provider:     ProviderName,
		FilesCreated: []string{},
		FilesSkipped: []string{},
		Warnings:     []string{},
		Metadata:     make(map[string]interface{}),
	}

	applyDryRunMetadata(result, "/tmp/config", "config-content", true, "/tmp/credentials", "creds-content")

	if result.Metadata["credentials_path"] != "/tmp/credentials" {
		t.Fatalf("credentials_path = %v", result.Metadata["credentials_path"])
	}
	if result.Metadata["credentials_content"] != "creds-content" {
		t.Fatalf("credentials_content = %v", result.Metadata["credentials_content"])
	}
}

func TestCheckExistingOutput(t *testing.T) {
	dir := t.TempDir()
	existingFile := filepath.Join(dir, "config")
	if err := os.WriteFile(existingFile, []byte("data"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	nonexistentFile := filepath.Join(dir, "nonexistent")

	tests := []struct {
		name     string
		path     string
		opts     *core.GenerateOptions
		expected bool
	}{
		{
			name:     "file exists no force no dryrun",
			path:     existingFile,
			opts:     &core.GenerateOptions{Force: false, DryRun: false},
			expected: true,
		},
		{
			name:     "file exists with force",
			path:     existingFile,
			opts:     &core.GenerateOptions{Force: true},
			expected: false,
		},
		{
			name:     "file exists with dryrun",
			path:     existingFile,
			opts:     &core.GenerateOptions{DryRun: true},
			expected: false,
		},
		{
			name:     "file does not exist",
			path:     nonexistentFile,
			opts:     &core.GenerateOptions{},
			expected: false,
		},
		{
			name:     "nil opts",
			path:     existingFile,
			opts:     nil,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &core.Result{
				FilesSkipped: []string{},
				Warnings:     []string{},
			}
			got := core.CheckExistingOutput(tt.path, tt.opts, result)
			if got != tt.expected {
				t.Fatalf("checkExistingOutput = %v, want %v", got, tt.expected)
			}
			if tt.expected {
				if len(result.FilesSkipped) == 0 {
					t.Fatal("expected file in FilesSkipped")
				}
				if len(result.Warnings) == 0 {
					t.Fatal("expected warning")
				}
			}
		})
	}
}

func TestApplyGenerateOptions(t *testing.T) {
	t.Run("nil config on provider", func(t *testing.T) {
		p := &Provider{config: nil}
		if err := p.applyGenerateOptions(nil); err == nil {
			t.Fatal("expected error for nil config")
		}
	})

	t.Run("nil opts", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.SSO.Region = testRegion
		cfg.SSO.StartURL = testStartURL
		cfg.TokenCachePaths = []string{t.TempDir()}
		p := NewProvider(cfg)
		if err := p.applyGenerateOptions(nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("opts with wrong config type", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.SSO.Region = testRegion
		cfg.SSO.StartURL = testStartURL
		cfg.TokenCachePaths = []string{t.TempDir()}
		p := NewProvider(cfg)
		opts := &core.GenerateOptions{Config: &wrongConfig{}}
		if err := p.applyGenerateOptions(opts); err == nil {
			t.Fatal("expected error for wrong config type")
		}
	})

	t.Run("opts with valid config", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.SSO.Region = testRegion
		cfg.SSO.StartURL = testStartURL
		cfg.TokenCachePaths = []string{t.TempDir()}
		p := NewProvider(cfg)

		newCfg := DefaultConfig()
		newCfg.SSO.Region = "us-west-2"
		newCfg.SSO.StartURL = testStartURL
		newCfg.TokenCachePaths = []string{t.TempDir()}
		opts := &core.GenerateOptions{Config: newCfg}
		if err := p.applyGenerateOptions(opts); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.config.SSO.Region != "us-west-2" {
			t.Fatalf("expected region us-west-2, got %q", p.config.SSO.Region)
		}
	})
}

func TestAllowCredentialsWrite(t *testing.T) {
	dir := t.TempDir()
	existingFile := filepath.Join(dir, "credentials")
	if err := os.WriteFile(existingFile, []byte("data"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	nonexistentFile := filepath.Join(dir, "nonexistent")

	tests := []struct {
		name     string
		enabled  bool
		path     string
		opts     *core.GenerateOptions
		expected bool
	}{
		{
			name:     "disabled",
			enabled:  false,
			path:     nonexistentFile,
			opts:     &core.GenerateOptions{Force: true},
			expected: false,
		},
		{
			name:     "enabled file not exist",
			enabled:  true,
			path:     nonexistentFile,
			opts:     &core.GenerateOptions{},
			expected: true,
		},
		{
			name:     "enabled file exists no force",
			enabled:  true,
			path:     existingFile,
			opts:     &core.GenerateOptions{Force: false},
			expected: false,
		},
		{
			name:     "enabled file exists with force",
			enabled:  true,
			path:     existingFile,
			opts:     &core.GenerateOptions{Force: true},
			expected: true,
		},
		{
			name:     "enabled file exists nil opts",
			enabled:  true,
			path:     existingFile,
			opts:     nil,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &core.Result{
				FilesSkipped: []string{},
				Warnings:     []string{},
			}
			got := allowCredentialsWrite(tt.enabled, tt.path, tt.opts, result)
			if got != tt.expected {
				t.Fatalf("allowCredentialsWrite = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWriteConfigFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config")
		if err := writeConfigFile(path, "test-content"); err != nil {
			t.Fatalf("writeConfigFile failed: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if string(data) != "test-content" {
			t.Fatalf("content = %q", string(data))
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		if err := writeConfigFile("/nonexistent/dir/config", "content"); err == nil {
			t.Fatal("expected error for invalid path")
		}
	})
}

func TestWriteCredentialsFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "subdir", "credentials")
		if err := writeCredentialsFile(path, "creds-content"); err != nil {
			t.Fatalf("writeCredentialsFile failed: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if string(data) != "creds-content" {
			t.Fatalf("content = %q", string(data))
		}
	})

	t.Run("creates parent directory", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "deep", "nested", "credentials")
		if err := writeCredentialsFile(path, "content"); err != nil {
			t.Fatalf("writeCredentialsFile failed: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file to exist: %v", err)
		}
	})
}

func TestProviderValidateNilConfig(t *testing.T) {
	provider := &Provider{config: nil}
	if err := provider.Validate(context.Background()); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestProviderValidateEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.SSO.Region = testRegion
	cfg.SSO.StartURL = testStartURL
	cfg.TokenCachePaths = []string{t.TempDir()}
	provider := NewProvider(cfg)

	if err := provider.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestProviderValidateConfigError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.SSO.Region = ""
	cfg.SSO.StartURL = testStartURL
	cfg.TokenCachePaths = []string{t.TempDir()}
	provider := NewProvider(cfg)

	if err := provider.Validate(context.Background()); err == nil {
		t.Fatal("expected validation error for missing region")
	}
}

func TestProviderGenerateDryRun(t *testing.T) {
	configDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.ConfigPath = filepath.Join(configDir, "config")
	cfg.SSO.Region = testRegion
	cfg.SSO.StartURL = testStartURL
	cfg.TokenCachePaths = []string{t.TempDir()}
	provider := NewProvider(cfg)
	provider.discover = func(context.Context, *Config) ([]DiscoveredProfile, error) {
		return []DiscoveredProfile{
			{AccountID: "111111111111", AccountName: "prod", RoleName: "Admin"},
		}, nil
	}

	result, err := provider.Generate(context.Background(), &core.GenerateOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if result.Metadata["config_path"] != cfg.ConfigPath {
		t.Fatalf("expected config_path in metadata, got %v", result.Metadata["config_path"])
	}
	if _, err := os.Stat(cfg.ConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expected no file to be created in dry-run mode")
	}
}
