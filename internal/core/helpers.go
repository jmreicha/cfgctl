package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// NewResult creates a zero-valued Result for a provider, with all slices
// initialized so callers can append without nil checks.
func NewResult(providerName string) *Result {
	return &Result{
		Provider:     providerName,
		FilesCreated: []string{},
		FilesSkipped: []string{},
		Warnings:     []string{},
		Metadata:     make(map[string]interface{}),
	}
}

// WriteConfigFile creates any missing parent directories then writes content
// to path with the given permission mode.
func WriteConfigFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directory for %q: %w", path, err)
	}
	return os.WriteFile(path, content, mode)
}

// CheckExistingOutput returns true when path exists and the caller should skip
// generation (file already present, no --force, not a dry-run). When it
// returns true it appends path to result.FilesSkipped and adds a warning.
func CheckExistingOutput(path string, opts *GenerateOptions, result *Result) bool {
	if opts != nil && (opts.Force || opts.DryRun) {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	result.FilesSkipped = append(result.FilesSkipped, path)
	result.Warnings = append(result.Warnings, "config file exists, use --force to overwrite")
	return true
}
