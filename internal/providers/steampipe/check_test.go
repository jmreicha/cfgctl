package steampipe

import (
	"context"
	"testing"

	"github.com/jmreicha/cfgctl/internal/core"
)

func TestSteampipeCheck_Disabled(t *testing.T) {
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

func TestSteampipeCheck_ServiceRunning(t *testing.T) {
	prev := steampipeCommand
	steampipeCommand = "true" // always exits 0
	t.Cleanup(func() { steampipeCommand = prev })

	p := NewProvider(DefaultConfig())
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
	if results[0].Target != "local" {
		t.Errorf("expected target %q, got %q", "local", results[0].Target)
	}
}

func TestSteampipeCheck_ServiceNotRunning(t *testing.T) {
	prev := steampipeCommand
	steampipeCommand = "false" // always exits 1
	t.Cleanup(func() { steampipeCommand = prev })

	p := NewProvider(DefaultConfig())
	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusFail {
		t.Errorf("expected Fail, got %v", results[0].Status)
	}
	if results[0].Note == "" {
		t.Error("expected non-empty note for failed service check")
	}
}
