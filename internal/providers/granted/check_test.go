package granted

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmreicha/cfgctl/internal/core"
)

func TestGrantedCheck_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	p := NewProvider(cfg)

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for disabled provider, got %d", len(results))
	}
}

func TestGrantedCheck_ConfigExists(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte("DefaultBrowser = STDOUT\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = configPath
	p := NewProvider(cfg)

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusOK {
		t.Errorf("expected OK, got %v", results[0].Status)
	}
}

func TestGrantedCheck_ConfigNotFound(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfigPath = t.TempDir() + "/nonexistent"
	p := NewProvider(cfg)

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusWarn {
		t.Errorf("expected Warn for missing config, got %v", results[0].Status)
	}
	if results[0].Note == "" {
		t.Error("expected non-empty note for missing config")
	}
}
