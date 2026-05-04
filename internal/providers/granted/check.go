package granted

import (
	"context"
	"os"

	"github.com/jmreicha/cfgctl/internal/core"
)

// Check verifies the generated granted config file is present.
func (p *Provider) Check(_ context.Context) ([]core.CheckResult, error) {
	if !p.config.Enabled {
		return nil, nil
	}

	if _, err := os.Stat(p.config.ConfigPath); err != nil {
		status := core.CheckStatusWarn
		note := "config not found — run 'cfgctl generate granted'"
		if !os.IsNotExist(err) {
			status = core.CheckStatusFail
			note = err.Error()
		}
		return []core.CheckResult{{
			Target: "config",
			Status: status,
			Note:   note,
		}}, nil
	}

	return []core.CheckResult{{
		Target: "config",
		Status: core.CheckStatusOK,
	}}, nil
}
