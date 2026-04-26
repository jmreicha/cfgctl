package core

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// ManagedConfig extends ProviderConfig with the lifecycle metadata that
// BaseProvider needs to implement the boilerplate Provider methods.
type ManagedConfig interface {
	ProviderConfig        // Validate() error
	EnabledProviderConfig // IsEnabled() bool
	// GetConfigPath returns the path of the primary config file managed by
	// this provider. Used by Backup, NeedsBackup, and Clean.
	GetConfigPath() string
}

// BaseProvider supplies default implementations for the boilerplate Provider
// methods (Validate, Backup, Restore, NeedsBackup, Clean). Embed it in a
// concrete provider struct and implement Name() and Generate().
type BaseProvider struct {
	name   string
	config ManagedConfig
}

// ConfigPather is an optional interface providers can satisfy to expose their
// primary config file path. BaseProvider implements it automatically.
type ConfigPather interface {
	ConfigPath() string
}

// NewBaseProvider creates a BaseProvider for the given provider name and config.
func NewBaseProvider(name string, cfg ManagedConfig) *BaseProvider {
	return &BaseProvider{name: name, config: cfg}
}

// ConfigPath returns the primary config file path for this provider.
func (b *BaseProvider) ConfigPath() string {
	if b == nil || b.config == nil {
		return ""
	}
	return b.config.GetConfigPath()
}

// Validate checks nil config, skips disabled providers, then delegates to
// config.Validate().
func (b *BaseProvider) Validate(_ context.Context) error {
	if b == nil || b.config == nil {
		return errors.New("provider config is nil")
	}
	if !b.config.IsEnabled() {
		return nil
	}
	return b.config.Validate()
}

// Backup creates a timestamped backup of the config file. Returns an empty
// path if no file exists yet.
func (b *BaseProvider) Backup(_ context.Context) (string, error) {
	if b == nil || b.config == nil {
		return "", nil
	}
	return BackupFile(b.config.GetConfigPath())
}

// Restore is not yet implemented. Providers that need restore logic should
// override this method.
func (b *BaseProvider) Restore(_ context.Context, _ string) error {
	return fmt.Errorf("restore not yet implemented for %q provider", b.name)
}

// NeedsBackup returns true when a backup is warranted: provider is enabled,
// not a dry-run, --force is set, and the config file already exists.
// If the file does not exist yet there is nothing to back up.
func (b *BaseProvider) NeedsBackup(opts *GenerateOptions) (bool, error) {
	if b == nil || b.config == nil {
		return false, nil
	}
	if opts != nil && opts.DryRun {
		return false, nil
	}
	if !b.config.IsEnabled() {
		return false, nil
	}
	// If generation will be skipped anyway (file exists, no --force), there's
	// nothing to overwrite so no backup is needed.
	if opts == nil || !opts.Force {
		if _, err := os.Stat(b.config.GetConfigPath()); err == nil {
			return false, nil
		}
	}
	// Backup only if the file actually exists.
	_, err := os.Stat(b.config.GetConfigPath())
	return err == nil, nil
}

// Clean removes the primary config file. Missing files are silently ignored.
func (b *BaseProvider) Clean(_ context.Context) error {
	if b == nil || b.config == nil {
		return nil
	}
	path := b.config.GetConfigPath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %q config: %w", b.name, err)
	}
	return nil
}
