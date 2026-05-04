package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmreicha/cfgctl/internal/core"
)

const warnThreshold = 2 * time.Hour

// Check verifies AWS SSO credential validity from the local token cache.
func (p *Provider) Check(_ context.Context) ([]core.CheckResult, error) {
	if !p.config.Enabled {
		return nil, nil
	}

	target := ssoTarget(p.config.SSO.StartURL)

	token, err := LoadMatchingToken(p.config.TokenCachePaths, p.config.SSO.StartURL, p.config.SSO.Region, time.Now())
	if err != nil {
		return []core.CheckResult{{ //nolint:nilerr // token error is surfaced as CheckStatusFail, not propagated
			Target: target,
			Status: core.CheckStatusFail,
			Note:   "no valid SSO token — run 'aws sso login'",
		}}, nil
	}

	remaining := time.Until(token.ExpiresAt)
	note := "expires in " + formatRemaining(remaining)

	status := core.CheckStatusOK
	if remaining < warnThreshold {
		status = core.CheckStatusWarn
	}

	return []core.CheckResult{{Target: target, Status: status, Note: note}}, nil
}

// ssoTarget reduces a start URL to a short display name.
func ssoTarget(startURL string) string {
	if startURL == "" {
		return "aws-sso"
	}
	if idx := strings.Index(startURL, "//"); idx >= 0 {
		host := startURL[idx+2:]
		if slash := strings.Index(host, "/"); slash >= 0 {
			host = host[:slash]
		}
		return host
	}
	return startURL
}

func formatRemaining(d time.Duration) string {
	if d <= 0 {
		return "expired"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
