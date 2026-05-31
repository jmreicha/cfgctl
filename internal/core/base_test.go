package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// managedConfigStub implements ManagedConfig for tests.
type managedConfigStub struct {
	enabled    bool
	configPath string
	validateFn func() error
}

func (s *managedConfigStub) Validate() error {
	if s.validateFn != nil {
		return s.validateFn()
	}
	return nil
}

func (s *managedConfigStub) IsEnabled() bool       { return s.enabled }
func (s *managedConfigStub) GetConfigPath() string { return s.configPath }

// --- NewResult ---

func TestNewResult(t *testing.T) {
	r := NewResult("myprovider")

	if r.Provider != "myprovider" {
		t.Errorf("Provider = %q, want %q", r.Provider, "myprovider")
	}
	if r.FilesCreated == nil {
		t.Error("FilesCreated is nil")
	}
	if r.FilesSkipped == nil {
		t.Error("FilesSkipped is nil")
	}
	if r.Warnings == nil {
		t.Error("Warnings is nil")
	}
	if r.Metadata == nil {
		t.Error("Metadata is nil")
	}
	if len(r.FilesCreated) != 0 || len(r.FilesSkipped) != 0 || len(r.Warnings) != 0 {
		t.Error("expected all slices to be empty")
	}
}

// --- WriteConfigFile ---

func TestWriteConfigFile_CreatesFileAndDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config")

	if err := WriteConfigFile(path, []byte("content"), 0600); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("content = %q, want %q", string(data), "content")
	}
}

func TestWriteConfigFile_SetsPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")

	if err := WriteConfigFile(path, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestWriteConfigFile_MkdirAllFails(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	_ = os.WriteFile(blocker, []byte("x"), 0600)

	path := filepath.Join(blocker, "subdir", "config")
	if err := WriteConfigFile(path, []byte("content"), 0600); err == nil {
		t.Error("expected error when parent path is a file")
	}
}

func TestWriteConfigFile_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	_ = os.WriteFile(path, []byte("old"), 0600)

	if err := WriteConfigFile(path, []byte("new"), 0600); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("expected overwrite, got %q", string(data))
	}
}

// --- CheckExistingOutput ---

func TestCheckExistingOutput_NoFile(t *testing.T) {
	result := NewResult("p")
	opts := &GenerateOptions{Force: false, DryRun: false}

	if CheckExistingOutput("/nonexistent/path", opts, result) {
		t.Error("expected false when file does not exist")
	}
	if len(result.FilesSkipped) != 0 {
		t.Error("expected no skipped files")
	}
}

func TestCheckExistingOutput_FileExistsNoForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	_ = os.WriteFile(path, []byte("x"), 0600)

	result := NewResult("p")
	opts := &GenerateOptions{Force: false, DryRun: false}

	if !CheckExistingOutput(path, opts, result) {
		t.Error("expected true when file exists and no --force")
	}
	if len(result.FilesSkipped) != 1 || result.FilesSkipped[0] != path {
		t.Errorf("FilesSkipped = %v, want [%s]", result.FilesSkipped, path)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected a warning")
	}
}

func TestCheckExistingOutput_FileExistsWithForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	_ = os.WriteFile(path, []byte("x"), 0600)

	result := NewResult("p")
	opts := &GenerateOptions{Force: true}

	if CheckExistingOutput(path, opts, result) {
		t.Error("expected false when --force is set")
	}
}

func TestCheckExistingOutput_FileExistsWithReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	_ = os.WriteFile(path, []byte("x"), 0600)

	result := NewResult("p")
	opts := &GenerateOptions{Replace: true}

	if CheckExistingOutput(path, opts, result) {
		t.Error("expected false when --replace is set")
	}
}

func TestCheckExistingOutput_DryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	_ = os.WriteFile(path, []byte("x"), 0600)

	result := NewResult("p")
	opts := &GenerateOptions{DryRun: true}

	if CheckExistingOutput(path, opts, result) {
		t.Error("expected false in dry-run even when file exists")
	}
}

func TestCheckExistingOutput_NilOpts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	_ = os.WriteFile(path, []byte("x"), 0600)

	result := NewResult("p")

	if !CheckExistingOutput(path, nil, result) {
		t.Error("expected true: nil opts means no force, file exists")
	}
}

// --- BaseProvider ---

func newTestBase(enabled bool, path string) *BaseProvider {
	return NewBaseProvider("test", &managedConfigStub{enabled: enabled, configPath: path})
}

func TestBaseProvider_Validate_NilConfig(t *testing.T) {
	b := &BaseProvider{name: "test", config: nil}
	if err := b.Validate(context.Background()); err == nil {
		t.Error("expected error for nil config")
	}
}

func TestBaseProvider_Validate_Disabled(t *testing.T) {
	b := newTestBase(false, "")
	if err := b.Validate(context.Background()); err != nil {
		t.Errorf("expected nil for disabled provider, got %v", err)
	}
}

func TestBaseProvider_Validate_ConfigError(t *testing.T) {
	cfg := &managedConfigStub{
		enabled:    true,
		validateFn: func() error { return errors.New("bad config") },
	}
	b := NewBaseProvider("test", cfg)
	if err := b.Validate(context.Background()); err == nil {
		t.Error("expected error from config.Validate()")
	}
}

func TestBaseProvider_Validate_OK(t *testing.T) {
	b := newTestBase(true, "")
	if err := b.Validate(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBaseProvider_Backup_NilConfig(t *testing.T) {
	b := &BaseProvider{name: "test", config: nil}
	path, err := b.Backup(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
}

func TestBaseProvider_Backup_NoFile(t *testing.T) {
	b := newTestBase(true, "/nonexistent/path")
	path, err := b.Backup(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path for missing file, got %q", path)
	}
}

func TestBaseProvider_Backup_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	_ = os.WriteFile(cfgPath, []byte("cfg"), 0600)

	b := newTestBase(true, cfgPath)
	path, err := b.Backup(context.Background())
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty backup path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("backup file missing: %v", err)
	}
}

func TestBaseProvider_Restore_ReturnsError(t *testing.T) {
	b := newTestBase(true, "")
	if err := b.Restore(context.Background(), "/some/path"); err == nil {
		t.Error("expected not-implemented error")
	}
}

func TestBaseProvider_NeedsBackup_NilConfig(t *testing.T) {
	b := &BaseProvider{name: "test", config: nil}
	needs, err := b.NeedsBackup(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if needs {
		t.Error("expected false for nil config")
	}
}

func TestBaseProvider_NeedsBackup_DryRun(t *testing.T) {
	b := newTestBase(true, "")
	needs, err := b.NeedsBackup(&GenerateOptions{DryRun: true})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if needs {
		t.Error("expected false for dry-run")
	}
}

func TestBaseProvider_NeedsBackup_Disabled(t *testing.T) {
	b := newTestBase(false, "")
	needs, err := b.NeedsBackup(&GenerateOptions{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if needs {
		t.Error("expected false when disabled")
	}
}

func TestBaseProvider_NeedsBackup_FileExistsNoForce(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	_ = os.WriteFile(cfgPath, []byte("x"), 0600)

	b := newTestBase(true, cfgPath)
	needs, err := b.NeedsBackup(&GenerateOptions{Force: false})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if needs {
		t.Error("expected false: file exists but no force means generation will skip")
	}
}

func TestBaseProvider_NeedsBackup_FileExistsWithForce(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	_ = os.WriteFile(cfgPath, []byte("x"), 0600)

	b := newTestBase(true, cfgPath)
	needs, err := b.NeedsBackup(&GenerateOptions{Force: true})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !needs {
		t.Error("expected true: file exists and --force will overwrite it")
	}
}

func TestBaseProvider_NeedsBackup_FileExistsWithReplace(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	_ = os.WriteFile(cfgPath, []byte("x"), 0600)

	b := newTestBase(true, cfgPath)
	needs, err := b.NeedsBackup(&GenerateOptions{Replace: true})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !needs {
		t.Error("expected true: file exists and --replace will overwrite it")
	}
}

func TestBaseProvider_NeedsBackup_NoFile(t *testing.T) {
	b := newTestBase(true, "/nonexistent/config")
	needs, err := b.NeedsBackup(&GenerateOptions{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if needs {
		t.Error("expected false: no file to back up")
	}
}

func TestBaseProvider_Clean_NilConfig(t *testing.T) {
	b := &BaseProvider{name: "test", config: nil}
	if err := b.Clean(context.Background()); err != nil {
		t.Errorf("expected nil for nil config, got %v", err)
	}
}

func TestBaseProvider_Clean_EmptyPath(t *testing.T) {
	b := newTestBase(true, "")
	if err := b.Clean(context.Background()); err != nil {
		t.Errorf("expected nil for empty path, got %v", err)
	}
}

func TestBaseProvider_Clean_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	_ = os.WriteFile(cfgPath, []byte("x"), 0600)

	b := newTestBase(true, cfgPath)
	if err := b.Clean(context.Background()); err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if _, err := os.Stat(cfgPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("expected file to be removed")
	}
}

func TestBaseProvider_Clean_RemoveError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	_ = os.MkdirAll(cfgPath, 0700)
	_ = os.WriteFile(filepath.Join(cfgPath, "child"), []byte("x"), 0600)

	b := newTestBase(true, cfgPath)
	if err := b.Clean(context.Background()); err == nil {
		t.Error("expected error when config path is a non-empty directory")
	}
}

func TestBaseProvider_Clean_MissingFile(t *testing.T) {
	b := newTestBase(true, "/nonexistent/config")
	if err := b.Clean(context.Background()); err != nil {
		t.Errorf("expected nil for missing file, got %v", err)
	}
}
