package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmreicha/cfgctl/internal/core"
)

func writeTokenFile(t *testing.T, dir, name string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
}

func tokenJSON(accessToken, expiresAt, startURL, region string) string {
	return `{"accessToken":"` + accessToken + `","expiresAt":"` + expiresAt + `","issuedAt":"2026-01-01T00:00:00Z","startUrl":"` + startURL + `","region":"` + region + `"}`
}

func TestAWSCheck_ValidToken(t *testing.T) {
	cache := t.TempDir()
	writeTokenFile(t, cache, "token.json", tokenJSON("tok", "2026-01-01T20:00:00Z", testStartURL, testRegion))

	cfg := DefaultConfig()
	cfg.SSO.StartURL = testStartURL
	cfg.SSO.Region = testRegion
	cfg.TokenCachePaths = []string{cache}
	p := NewProvider(cfg)

	// Freeze time so the token is valid and not expiring soon.
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	results := checkWithTime(p, now)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusOK {
		t.Errorf("expected OK, got %v (note: %s)", results[0].Status, results[0].Note)
	}
}

func TestAWSCheck_ExpiringToken(t *testing.T) {
	cache := t.TempDir()
	// Token expires in 1 hour — below the 2-hour warn threshold.
	writeTokenFile(t, cache, "token.json", tokenJSON("tok", "2026-01-01T13:00:00Z", testStartURL, testRegion))

	cfg := DefaultConfig()
	cfg.SSO.StartURL = testStartURL
	cfg.SSO.Region = testRegion
	cfg.TokenCachePaths = []string{cache}
	p := NewProvider(cfg)

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	results := checkWithTime(p, now)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusWarn {
		t.Errorf("expected Warn, got %v", results[0].Status)
	}
}

func TestAWSCheck_ExpiredToken(t *testing.T) {
	cache := t.TempDir()
	// Token already expired.
	writeTokenFile(t, cache, "token.json", tokenJSON("tok", "2026-01-01T11:00:00Z", testStartURL, testRegion))

	cfg := DefaultConfig()
	cfg.SSO.StartURL = testStartURL
	cfg.SSO.Region = testRegion
	cfg.TokenCachePaths = []string{cache}
	p := NewProvider(cfg)

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	results := checkWithTime(p, now)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusFail {
		t.Errorf("expected Fail, got %v", results[0].Status)
	}
}

func TestAWSCheck_NoTokenCache(t *testing.T) {
	cache := t.TempDir() // empty — no token files

	cfg := DefaultConfig()
	cfg.SSO.StartURL = testStartURL
	cfg.SSO.Region = testRegion
	cfg.TokenCachePaths = []string{cache}
	p := NewProvider(cfg)

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	results := checkWithTime(p, now)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusFail {
		t.Errorf("expected Fail for empty cache, got %v", results[0].Status)
	}
}

func TestAWSCheck_Disabled(t *testing.T) {
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

func TestAWSCheck_EmptyStartURL(t *testing.T) {
	cache := t.TempDir()

	cfg := DefaultConfig()
	cfg.SSO.StartURL = ""
	cfg.SSO.Region = testRegion
	cfg.TokenCachePaths = []string{cache}
	p := NewProvider(cfg)

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	results := checkWithTime(p, now)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Target == "" {
		t.Error("expected non-empty target even with no start URL")
	}
}

func TestSSOTarget(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.awsapps.com/start", "example.awsapps.com"},
		{"https://foo.bar.com/start#hash", "foo.bar.com"},
		{"", "aws-sso"},
		{"nodoubledash", "nodoubledash"},
	}
	for _, tt := range tests {
		got := ssoTarget(tt.input)
		if got != tt.want {
			t.Errorf("ssoTarget(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatRemaining(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{2*time.Hour + 30*time.Minute, "2h30m"},
		{45 * time.Minute, "45m"},
		{0, "expired"},
		{-time.Minute, "expired"},
	}
	for _, tt := range tests {
		got := formatRemaining(tt.d)
		if got != tt.want {
			t.Errorf("formatRemaining(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// checkWithTime runs a check using a fixed reference time to make token expiry deterministic.
// Token errors are converted to a CheckStatusFail result rather than propagated.
func checkWithTime(p *Provider, now time.Time) []core.CheckResult {
	token, err := LoadMatchingToken(p.config.TokenCachePaths, p.config.SSO.StartURL, p.config.SSO.Region, now)
	if err != nil {
		return []core.CheckResult{{
			Target: ssoTarget(p.config.SSO.StartURL),
			Status: core.CheckStatusFail,
			Note:   "no valid SSO token — run 'aws sso login'",
		}}
	}

	remaining := token.ExpiresAt.Sub(now)
	note := "expires in " + formatRemaining(remaining)
	status := core.CheckStatusOK
	if remaining < warnThreshold {
		status = core.CheckStatusWarn
	}
	return []core.CheckResult{{Target: ssoTarget(p.config.SSO.StartURL), Status: status, Note: note}}
}
