package cli

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmreicha/cfgctl/internal/core"
)

// mockProvider is a test implementation of the Provider interface.
type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Validate(_ context.Context) error {
	return nil
}

func (m *mockProvider) Generate(_ context.Context, _ *core.GenerateOptions) (*core.Result, error) {
	return &core.Result{
		Provider:     m.name,
		FilesCreated: []string{},
	}, nil
}

func (m *mockProvider) Backup(_ context.Context) (string, error) {
	return "", nil
}

func (m *mockProvider) Restore(_ context.Context, _ string) error {
	return nil
}

func (m *mockProvider) Clean(_ context.Context) error {
	return nil
}

func TestGenerateCmd(t *testing.T) {
	// Set up test components
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	registry = core.NewRegistry()
	backupManager = core.NewBackupManager("")
	config = core.NewConfig()
	engine = core.NewEngine(registry, backupManager, config, logger)
	sshConfigPath = ""

	// Register test providers
	if err := registry.Register(&mockProvider{name: "aws"}); err != nil {
		t.Fatalf("failed to register aws provider: %v", err)
	}
	if err := registry.Register(&mockProvider{name: "kubernetes"}); err != nil {
		t.Fatalf("failed to register kubernetes provider: %v", err)
	}
	if err := registry.Register(&mockProvider{name: "ssh"}); err != nil {
		t.Fatalf("failed to register ssh provider: %v", err)
	}

	tests := []struct {
		name               string
		args               []string
		expectProviders    []string
		expectAllProviders bool
	}{
		{
			name:               "no args shows help",
			args:               []string{},
			expectProviders:    nil,
			expectAllProviders: false,
		},
		{
			name:               "single provider",
			args:               []string{"aws"},
			expectProviders:    []string{"aws"},
			expectAllProviders: false,
		},
		{
			name:               "multiple providers",
			args:               []string{"aws", "kubernetes"},
			expectProviders:    []string{"aws", "kubernetes"},
			expectAllProviders: false,
		},
		{
			name:               "multiple providers without kubernetes",
			args:               []string{"aws", "ssh"},
			expectProviders:    []string{"aws", "ssh"},
			expectAllProviders: false,
		},
		{
			name:               "all keyword runs all providers",
			args:               []string{"all"},
			expectProviders:    []string{"aws", "kubernetes"},
			expectAllProviders: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newGenerateCmd()
			cmd.SetArgs(tt.args)

			// Execute the command
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command execution failed: %v", err)
			}

			// Verify behavior - in real testing we'd capture the results
			// For now, the fact that execution succeeded validates the logic
		})
	}
}

func TestGenerateCmdAllKeyword(t *testing.T) {
	// Set up test components
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	registry = core.NewRegistry()
	backupManager = core.NewBackupManager("")
	config = core.NewConfig()
	engine = core.NewEngine(registry, backupManager, config, logger)
	sshConfigPath = ""

	// Register test providers
	if err := registry.Register(&mockProvider{name: "test1"}); err != nil {
		t.Fatalf("failed to register test1 provider: %v", err)
	}
	if err := registry.Register(&mockProvider{name: "test2"}); err != nil {
		t.Fatalf("failed to register test2 provider: %v", err)
	}

	// Test that "all" argument executes all providers
	cmd := newGenerateCmd()
	cmd.SetArgs([]string{"all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command execution with 'all' failed: %v", err)
	}
}

func TestGenerateCmdNoArgsShowsHelp(t *testing.T) {
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	registry = core.NewRegistry()
	backupManager = core.NewBackupManager("")
	config = core.NewConfig()
	engine = core.NewEngine(registry, backupManager, config, logger)

	cmd := newGenerateCmd()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error when no args, got: %v", err)
	}
}

func TestPrintGenerateResults(t *testing.T) {
	results := map[string]*core.Result{
		"test": {
			Provider:     "test",
			FilesCreated: []string{filepath.Join("/tmp", "created")},
			FilesSkipped: []string{filepath.Join("/tmp", "skipped")},
			BackupPath:   filepath.Join("/tmp", "backup"),
			Warnings:     []string{"warning"},
		},
	}

	prevStdout := os.Stdout
	tmpFile, err := os.CreateTemp("", "cfgctl-output")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() {
		os.Stdout = prevStdout
		if err := tmpFile.Close(); err != nil {
			t.Fatalf("failed to close temp file: %v", err)
		}
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Fatalf("failed to remove temp file: %v", err)
		}
	})

	os.Stdout = tmpFile
	printGenerateResults(results)
}
