package steampipe

import (
	"context"
	"os/exec"
	"time"

	"github.com/jmreicha/cfgctl/internal/core"
)

// steampipeCommand is the steampipe binary name. Overridden in tests.
var steampipeCommand = "steampipe"

// Check verifies the steampipe service is running.
func (p *Provider) Check(ctx context.Context) ([]core.CheckResult, error) {
	if !p.config.Enabled {
		return nil, nil
	}

	start := time.Now()
	// #nosec G204 -- steampipeCommand is a fixed binary name, not user input.
	cmd := exec.CommandContext(ctx, steampipeCommand, "service", "status")
	err := cmd.Run()
	latency := time.Since(start)

	if err != nil {
		return []core.CheckResult{{ //nolint:nilerr // exec error is surfaced as CheckStatusFail, not propagated
			Target: "local",
			Status: core.CheckStatusFail,
			Note:   "service not running — start with 'steampipe service start'",
		}}, nil
	}

	return []core.CheckResult{{
		Target:  "local",
		Status:  core.CheckStatusOK,
		Latency: latency,
	}}, nil
}
