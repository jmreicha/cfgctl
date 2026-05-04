package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/jmreicha/cfgctl/internal/core"
)

const defaultSSHPort = 22

// Check verifies TCP connectivity to each configured SSH host.
func (p *Provider) Check(ctx context.Context) ([]core.CheckResult, error) {
	if !p.config.Enabled {
		return nil, nil
	}

	results := make([]core.CheckResult, 0, len(p.config.Hosts))
	for _, host := range p.config.Hosts {
		results = append(results, dialHost(ctx, host))
	}
	return results, nil
}

func dialHost(ctx context.Context, host HostConfig) core.CheckResult {
	target := host.Host
	hostname := host.Hostname
	if hostname == "" {
		hostname = host.Host
	}

	port := host.Port
	if port <= 0 {
		port = defaultSSHPort
	}

	addr := fmt.Sprintf("%s:%d", hostname, port)
	var d net.Dialer
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", addr)
	latency := time.Since(start)
	if err != nil {
		return core.CheckResult{
			Target: target,
			Status: core.CheckStatusFail,
			Note:   friendlyDialError(err),
		}
	}
	_ = conn.Close()

	return core.CheckResult{
		Target:  target,
		Status:  core.CheckStatusOK,
		Latency: latency,
	}
}

func friendlyDialError(err error) string {
	var netErr *net.OpError
	if ok := isNetOpError(err, &netErr); ok {
		if netErr.Timeout() {
			return "timed out"
		}
		if netErr.Op == "dial" {
			return "connection refused"
		}
	}
	return err.Error()
}

func isNetOpError(err error, target **net.OpError) bool {
	v := &net.OpError{}
	ok := errors.As(err, &v)
	if ok {
		*target = v
	}
	return ok
}
