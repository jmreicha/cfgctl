package kubernetes

import (
	"context"
	"testing"

	"github.com/jmreicha/cfgctl/internal/core"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestKubernetesCheck_Disabled(t *testing.T) {
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

func TestKubernetesCheck_EmptyConfigPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfigPath = ""
	p := NewProvider(cfg)

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusFail {
		t.Errorf("expected Fail for empty config path, got %v", results[0].Status)
	}
}

func TestKubernetesCheck_FileNotFound(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfigPath = t.TempDir() + "/nonexistent-kubeconfig"
	p := NewProvider(cfg)

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusWarn {
		t.Errorf("expected Warn for missing file, got %v", results[0].Status)
	}
	if results[0].Note == "" {
		t.Error("expected non-empty note for missing file")
	}
}

func TestKubernetesCheck_EmptyKubeconfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfigPath = writeTestKubeconfig(t, clientcmdapi.NewConfig())
	p := NewProvider(cfg)

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for empty kubeconfig, got %d", len(results))
	}
}

func TestKubernetesCheck_ContextCount(t *testing.T) {
	raw := clientcmdapi.NewConfig()
	raw.Clusters["c1"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:1"}
	raw.Clusters["c2"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:2"}
	raw.AuthInfos["u1"] = &clientcmdapi.AuthInfo{}
	raw.AuthInfos["u2"] = &clientcmdapi.AuthInfo{}
	raw.Contexts["ctx1"] = &clientcmdapi.Context{Cluster: "c1", AuthInfo: "u1"}
	raw.Contexts["ctx2"] = &clientcmdapi.Context{Cluster: "c2", AuthInfo: "u2"}

	cfg := DefaultConfig()
	cfg.ConfigPath = writeTestKubeconfig(t, raw)
	p := NewProvider(cfg)

	// Both contexts point to unreachable addresses — we expect two Fail results,
	// one per context, not an error from Check itself.
	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results (one per context), got %d", len(results))
	}
	for _, r := range results {
		if r.Target == "" {
			t.Error("expected non-empty target for each result")
		}
		if r.Status != core.CheckStatusFail {
			t.Errorf("expected Fail for unreachable context %q, got %v", r.Target, r.Status)
		}
	}
}

func TestFriendlyKubeError(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"x509: certificate signed by unknown authority", "certificate error"},
		{"dial tcp: connection refused", "connection refused"},
		{"context deadline exceeded", "timed out"},
		{"no such host", "host not found"},
		{"Unauthorized", "unauthorized"},
		{"401 Unauthorized", "unauthorized"},
		{"some unknown error", "some unknown error"},
	}
	for _, tt := range tests {
		got := friendlyKubeError(checkError(tt.msg))
		if got != tt.want {
			t.Errorf("friendlyKubeError(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

// checkError is a minimal error implementation for table-driven tests.
type checkError string

func (e checkError) Error() string { return string(e) }

// writeTestKubeconfig writes a kubeconfig to a temp file and returns its path.
func writeTestKubeconfig(t *testing.T, cfg *clientcmdapi.Config) string {
	t.Helper()
	path := t.TempDir() + "/kubeconfig"
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatalf("write test kubeconfig: %v", err)
	}
	return path
}
