package ssh

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
				Enabled:    true,
				ConfigPath: "/home/user/.ssh",
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

func TestConfig_ValidateBasic(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				ConfigPath: "/home/user/.ssh",
				GlobalOptions: map[string]string{
					"ServerAliveInterval": "60",
				},
				Hosts: []HostConfig{
					{
						Host:     "example.com",
						Hostname: "example.com",
						User:     "user",
					},
				},
			},
			expectError: false,
		},
		{
			name: "empty config path",
			config: &Config{
				ConfigPath: "",
			},
			expectError: true,
		},
		{
			name: "empty host pattern",
			config: &Config{
				ConfigPath: "/home/user/.ssh",
				Hosts: []HostConfig{
					{
						Host:     "",
						Hostname: "example.com",
					},
				},
			},
			expectError: true,
		},
		{
			name: "no hosts",
			config: &Config{
				ConfigPath: "/home/user/.ssh",
				Hosts:      []HostConfig{},
			},
			expectError: false,
		},
		{
			name: "global only",
			config: &Config{
				ConfigPath: "/home/user/.ssh",
				GlobalOptions: map[string]string{
					"ServerAliveInterval": "60",
				},
				Hosts: []HostConfig{},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
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
				ConfigPath: "/home/user/.ssh",
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
			name: "invalid config",
			provider: NewProvider(&Config{
				Enabled:    true,
				ConfigPath: "",
			}),
			expectError: true,
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
	provider := NewProvider(&Config{
		ConfigPath:    "/home/user/.ssh",
		GlobalOptions: map[string]string{"ServerAliveInterval": "60"},
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

func TestProvider_Backup(t *testing.T) {
	provider := NewProvider(&Config{
		ConfigPath:    "/home/user/.ssh",
		GlobalOptions: map[string]string{"ServerAliveInterval": "60"},
	})

	ctx := context.Background()
	backupPath, err := provider.Backup(ctx)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Skeleton implementation returns empty string
	if backupPath != "" {
		t.Errorf("backupPath = %q, want empty string", backupPath)
	}
}

func TestProvider_Restore(t *testing.T) {
	provider := NewProvider(&Config{
		ConfigPath:    "/home/user/.ssh",
		GlobalOptions: map[string]string{"ServerAliveInterval": "60"},
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
			expectError: true, // Not yet implemented
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
		ConfigPath:    "/home/user/.ssh",
		GlobalOptions: map[string]string{"ServerAliveInterval": "60"},
	})

	ctx := context.Background()
	err := provider.Clean(ctx)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProvider_InterfaceCompliance(_ *testing.T) {
	// Compile-time check that Provider implements core.Provider
	var _ core.Provider = (*Provider)(nil)
}

// TestProvider_GenerateIntegration tests the full generation lifecycle with real filesystem.
func TestProvider_GenerateIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	provider := NewProvider(&Config{
		Enabled:    true,
		ConfigPath: tmpDir,
		GlobalOptions: map[string]string{
			"ServerAliveInterval": "60",
		},
		Hosts: []HostConfig{
			{
				Host:     "test1.example.com",
				Hostname: "192.168.1.10",
				User:     "testuser",
				Port:     22,
			},
			{
				Host:     "test2.example.com",
				Hostname: "192.168.1.20",
				User:     "admin",
				Port:     2222,
			},
		},
	})

	ctx := context.Background()

	// Test validation
	err := provider.Validate(ctx)
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// Test generation
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

	// Verify file was created
	configPath := tmpDir + "/config"
	if len(result.FilesCreated) != 1 || result.FilesCreated[0] != configPath {
		t.Errorf("expected config file to be created at %s", configPath)
	}

	// Verify file exists
	info, err := os.Stat(configPath)
	if err != nil {
		t.Errorf("config file not found: %v", err)
	}

	// Verify permissions
	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}

	// Parse and verify content
	cfg, err := ParseConfig(configPath)
	if err != nil {
		t.Fatalf("failed to parse generated config: %v", err)
	}

	// Verify hosts
	host1 := FindHost(cfg, "test1.example.com")
	if host1 == nil {
		t.Error("test1.example.com not found in generated config")
	}

	host2 := FindHost(cfg, "test2.example.com")
	if host2 == nil {
		t.Error("test2.example.com not found in generated config")
	}

	// Verify global options
	interval, err := cfg.Get("anyhost.com", "ServerAliveInterval")
	if err != nil {
		t.Errorf("failed to get ServerAliveInterval: %v", err)
	}
	if interval != "60" {
		t.Errorf("ServerAliveInterval = %s, want 60", interval)
	}
}

// TestProvider_GenerateWithExistingConfig tests updating an existing config.
func TestProvider_GenerateWithExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config"

	// Create existing config
	existingConfig := `Host existing.com
    HostName existing.example.com
    User existing

Host *
    ServerAliveInterval 30
`
	if err := os.WriteFile(configPath, []byte(existingConfig), 0600); err != nil {
		t.Fatalf("failed to create existing config: %v", err)
	}

	provider := NewProvider(&Config{
		Enabled:    true,
		ConfigPath: tmpDir,
		Hosts: []HostConfig{
			{
				Host:     "new.com",
				Hostname: "new.example.com",
				User:     "newuser",
				Port:     22,
			},
		},
	})

	ctx := context.Background()

	// Test generation without force (should skip)
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

	// Test generation with force
	opts.Force = true

	result, err = provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("generation with force failed: %v", err)
	}

	if len(result.FilesCreated) != 1 {
		t.Error("expected file to be created with force")
	}

	// Verify both old and new hosts exist
	cfg, err := ParseConfig(configPath)
	if err != nil {
		t.Fatalf("failed to parse generated config: %v", err)
	}

	existingHost := FindHost(cfg, "existing.com")
	if existingHost == nil {
		t.Error("existing.com host was lost")
	}

	newHost := FindHost(cfg, "new.com")
	if newHost == nil {
		t.Error("new.com host was not added")
	}
}

// TestProvider_GenerateDryRun tests dry-run mode.
func TestProvider_GenerateDryRun(t *testing.T) {
	tmpDir := t.TempDir()

	provider := NewProvider(&Config{
		Enabled:    true,
		ConfigPath: tmpDir,
		Hosts: []HostConfig{
			{
				Host:     "test.com",
				Hostname: "test.example.com",
				User:     "testuser",
				Port:     22,
			},
		},
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

	// Verify no files were created
	configPath := tmpDir + "/config"
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("config file should not exist in dry-run mode")
	}

	// Verify warnings
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

	// Verify metadata includes hosts to add
	if _, ok := result.Metadata["hosts_to_add"]; !ok {
		t.Error("expected hosts_to_add in metadata")
	}
}

// TestProvider_GenerateDisabled tests behavior when provider is disabled.
func TestProvider_GenerateDisabled(t *testing.T) {
	provider := NewProvider(&Config{
		Enabled:    false,
		ConfigPath: "/tmp",
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

// TestProvider_GenerateNoHosts tests behavior with no hosts configured.
func TestProvider_GenerateNoHosts(t *testing.T) {
	tmpDir := t.TempDir()

	provider := NewProvider(&Config{
		Enabled:    true,
		ConfigPath: tmpDir,
		Hosts:      []HostConfig{},
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
		t.Error("expected no files to be created with no hosts")
	}

	foundNoHostsWarning := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "no hosts") {
			foundNoHostsWarning = true
			break
		}
	}
	if !foundNoHostsWarning {
		t.Error("expected 'no hosts' warning in results")
	}
}

// TestProvider_GenerateInvalidConfig tests handling of invalid configuration.
func TestProvider_GenerateInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "empty config path",
			config: &Config{
				Enabled:    true,
				ConfigPath: "",
			},
		},
		{
			name: "invalid host config",
			config: &Config{
				Enabled:    true,
				ConfigPath: "/tmp",
				Hosts: []HostConfig{
					{
						Host: "", // Empty host pattern
					},
				},
			},
		},
		{
			name: "invalid port",
			config: &Config{
				Enabled:    true,
				ConfigPath: "/tmp",
				Hosts: []HostConfig{
					{
						Host: "test.com",
						Port: -1,
					},
				},
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

//nolint:dupl // intentional near-duplicate: tests Replace:true without Force
func TestGenerateReplaceWithoutForce(t *testing.T) {
	ctx := context.Background()
	sshDir := t.TempDir()
	configPath := filepath.Join(sshDir, "config")

	// Write an existing SSH config with a manual host not in cfgctl config.
	existing := `Host manual-host
  HostName 10.0.0.99
  User admin
`
	if err := os.WriteFile(configPath, []byte(existing), 0600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = sshDir
	cfg.Hosts = []HostConfig{
		{Host: "managed-host", Hostname: "10.0.0.1", User: "ubuntu"},
	}

	provider := NewProvider(cfg)
	opts := &core.GenerateOptions{
		Force:   false,
		Replace: true,
	}
	result, err := provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(result.FilesCreated) == 0 {
		t.Fatal("expected files created; Replace:true should not skip existing file")
	}

	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(written), "manual-host") {
		t.Fatalf("manual-host should have been dropped with --replace, got:\n%s", written)
	}
	if !strings.Contains(string(written), "managed-host") {
		t.Fatalf("managed-host should be present, got:\n%s", written)
	}
}

//nolint:dupl // intentional near-duplicate: tests Replace:true with Force:true
func TestGenerateReplaceDropsManualHosts(t *testing.T) {
	ctx := context.Background()
	sshDir := t.TempDir()
	configPath := filepath.Join(sshDir, "config")

	// Write an existing SSH config with a manual host not in cfgctl config.
	existing := `Host manual-host
  HostName 10.0.0.99
  User admin
`
	if err := os.WriteFile(configPath, []byte(existing), 0600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = sshDir
	cfg.Hosts = []HostConfig{
		{Host: "managed-host", Hostname: "10.0.0.1", User: "ubuntu"},
	}

	provider := NewProvider(cfg)
	opts := &core.GenerateOptions{
		Force:   true,
		Replace: true,
	}
	result, err := provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(result.FilesCreated) == 0 {
		t.Fatal("expected files created")
	}

	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(written), "manual-host") {
		t.Fatalf("manual-host should have been dropped, got:\n%s", written)
	}
	if !strings.Contains(string(written), "managed-host") {
		t.Fatalf("managed-host should be present, got:\n%s", written)
	}
}

// TestProvider_ValidateInvalidPaths tests validation with invalid paths.
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
			configPath:  "/tmp/ssh",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(&Config{
				Enabled:    true,
				ConfigPath: tt.configPath,
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
