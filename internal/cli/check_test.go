package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmreicha/cfgctl/internal/core"
)

const dashLatency = "—"

// checkableProvider implements core.Provider + core.Checker for check command tests.
type checkableProvider struct {
	commandProvider
	results  []core.CheckResult
	checkErr error
}

func (p *checkableProvider) Check(_ context.Context) ([]core.CheckResult, error) {
	return p.results, p.checkErr
}

func TestBuildCheckRows_NilResults(t *testing.T) {
	rows, anyFail := buildCheckRows([]core.ProviderCheckResults{
		{Provider: "aws", Results: nil},
	})
	if len(rows) != 0 {
		t.Errorf("expected no rows for nil results (disabled provider), got %d", len(rows))
	}
	if anyFail {
		t.Error("expected anyFail=false for nil results")
	}
}

func TestBuildCheckRows_EmptyResults(t *testing.T) {
	rows, anyFail := buildCheckRows([]core.ProviderCheckResults{
		{Provider: "ssh", Results: []core.CheckResult{}},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for empty results (unconfigured), got %d", len(rows))
	}
	if !rows[0].unconfigured {
		t.Error("expected unconfigured=true")
	}
	if anyFail {
		t.Error("expected anyFail=false for unconfigured provider")
	}
}

func TestBuildCheckRows_FailResult(t *testing.T) {
	rows, anyFail := buildCheckRows([]core.ProviderCheckResults{
		{Provider: "aws", Results: []core.CheckResult{
			{Target: "example.awsapps.com", Status: core.CheckStatusFail, Note: "no token"},
		}},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !anyFail {
		t.Error("expected anyFail=true for CheckStatusFail result")
	}
	if rows[0].note != "no token" {
		t.Errorf("expected note %q, got %q", "no token", rows[0].note)
	}
}

func TestBuildCheckRows_ProviderError(t *testing.T) {
	rows, anyFail := buildCheckRows([]core.ProviderCheckResults{
		{Provider: "aws", Err: errors.New("boom")},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for provider error, got %d", len(rows))
	}
	if rows[0].err != "boom" {
		t.Errorf("expected err=%q, got %q", "boom", rows[0].err)
	}
	if !anyFail {
		t.Error("expected anyFail=true for provider error")
	}
}

func TestBuildCheckRows_Latency(t *testing.T) {
	rows, _ := buildCheckRows([]core.ProviderCheckResults{
		{Provider: "ssh", Results: []core.CheckResult{
			{Target: "host", Status: core.CheckStatusOK, Latency: 42 * time.Millisecond},
		}},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].latency == dashLatency {
		t.Error("expected non-dash latency for non-zero Latency")
	}
}

func TestBuildCheckRows_ZeroLatency(t *testing.T) {
	rows, _ := buildCheckRows([]core.ProviderCheckResults{
		{Provider: "aws", Results: []core.CheckResult{
			{Target: "sso", Status: core.CheckStatusOK, Latency: 0},
		}},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].latency != dashLatency {
		t.Errorf("expected dash latency for zero Latency, got %q", rows[0].latency)
	}
}

func TestStatusDisplay(t *testing.T) {
	tests := []struct {
		status core.CheckStatus
		isErr  bool
		label  string
	}{
		{core.CheckStatusOK, false, "OKAY"},
		{core.CheckStatusWarn, false, "WARN"},
		{core.CheckStatusFail, false, "ERROR"},
		{core.CheckStatusOK, true, "ERROR"},
	}
	for _, tt := range tests {
		label, _ := statusDisplay(tt.status, tt.isErr)
		if label != tt.label {
			t.Errorf("statusDisplay(%v, isErr=%v) = %q, want %q", tt.status, tt.isErr, label, tt.label)
		}
	}
}

func TestCheckCmd_ExitsNonZeroOnFail(t *testing.T) {
	p := &checkableProvider{
		commandProvider: commandProvider{name: "aws"},
		results: []core.CheckResult{
			{Target: "test", Status: core.CheckStatusFail, Note: "expired"},
		},
	}
	setupCommandEngine(t, p)
	if err := newCheckCmd().Execute(); err == nil {
		t.Error("expected non-nil error when a check fails")
	}
}

func TestCheckCmd_ExitsZeroOnOK(t *testing.T) {
	p := &checkableProvider{
		commandProvider: commandProvider{name: "aws"},
		results: []core.CheckResult{
			{Target: "test", Status: core.CheckStatusOK},
		},
	}
	setupCommandEngine(t, p)
	if err := newCheckCmd().Execute(); err != nil {
		t.Errorf("expected nil error when all checks pass, got %v", err)
	}
}

func TestCheckCmd_NoProviders(t *testing.T) {
	setupCommandEngine(t) // no providers registered
	if err := newCheckCmd().Execute(); err != nil {
		t.Errorf("expected nil error with no providers, got %v", err)
	}
}

func TestCheckCmd_ProviderError(t *testing.T) {
	p := &checkableProvider{
		commandProvider: commandProvider{name: "aws"},
		checkErr:        errors.New("internal error"),
	}
	setupCommandEngine(t, p)
	if err := newCheckCmd().Execute(); err == nil {
		t.Error("expected non-nil error when provider Check returns an error")
	}
}
