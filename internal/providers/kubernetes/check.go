package kubernetes

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jmreicha/cfgctl/internal/core"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const checkConcurrency = 5

// Check verifies connectivity to each context in the kubeconfig.
func (p *Provider) Check(ctx context.Context) ([]core.CheckResult, error) {
	if !p.config.Enabled {
		return nil, nil
	}

	configPath := p.config.ConfigPath
	if configPath == "" {
		return []core.CheckResult{{
			Target: "kubeconfig",
			Status: core.CheckStatusFail,
			Note:   "config path not set",
		}}, nil
	}

	rawConfig, err := clientcmd.LoadFromFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []core.CheckResult{{
				Target: "kubeconfig",
				Status: core.CheckStatusWarn,
				Note:   "file not found — run 'cfgctl generate kubernetes'",
			}}, nil
		}
		return nil, err
	}

	contexts := make([]string, 0, len(rawConfig.Contexts))
	for name := range rawConfig.Contexts {
		contexts = append(contexts, name)
	}
	sort.Strings(contexts)

	sem := make(chan struct{}, checkConcurrency)
	results := make([]core.CheckResult, len(contexts))
	var wg sync.WaitGroup

	for i, name := range contexts {
		wg.Add(1)
		go func(idx int, contextName string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = pingContext(ctx, rawConfig, contextName)
		}(i, name)
	}

	wg.Wait()
	return results, nil
}

func pingContext(_ context.Context, rawConfig *clientcmdapi.Config, contextName string) core.CheckResult {
	clientConfig := clientcmd.NewNonInteractiveClientConfig(
		*rawConfig, contextName, &clientcmd.ConfigOverrides{}, nil,
	)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return core.CheckResult{
			Target: contextName,
			Status: core.CheckStatusFail,
			Note:   err.Error(),
		}
	}

	dc, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return core.CheckResult{
			Target: contextName,
			Status: core.CheckStatusFail,
			Note:   err.Error(),
		}
	}

	start := time.Now()
	_, err = dc.ServerVersion()
	latency := time.Since(start)
	if err != nil {
		return core.CheckResult{
			Target: contextName,
			Status: core.CheckStatusFail,
			Note:   friendlyKubeError(err),
		}
	}

	return core.CheckResult{
		Target:  contextName,
		Status:  core.CheckStatusOK,
		Latency: latency,
	}
}

func friendlyKubeError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "certificate"):
		return "certificate error"
	case strings.Contains(msg, "connection refused"):
		return "connection refused"
	case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "context deadline exceeded"):
		return "timed out"
	case strings.Contains(msg, "no such host"):
		return "host not found"
	case strings.Contains(msg, "Unauthorized"), strings.Contains(msg, "401"):
		return "unauthorized"
	default:
		return msg
	}
}
