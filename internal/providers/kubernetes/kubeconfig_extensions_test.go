package kubernetes

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func TestIsCfgctlManaged(t *testing.T) {
	tests := []struct {
		name       string
		extensions map[string]runtime.Object
		want       bool
	}{
		{
			name:       "nil extensions",
			extensions: nil,
			want:       false,
		},
		{
			name:       "empty extensions",
			extensions: map[string]runtime.Object{},
			want:       false,
		},
		{
			name: "unrelated extension",
			extensions: map[string]runtime.Object{
				"other.io/metadata": &runtime.Unknown{},
			},
			want: false,
		},
		{
			name: "cfgctl extension present",
			extensions: map[string]runtime.Object{
				cfgctlExtensionKey: newCfgctlExtension("test"),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCfgctlManaged(tt.extensions); got != tt.want {
				t.Errorf("isCfgctlManaged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStampCfgctlExtensions(t *testing.T) {
	config := newKubeconfig()
	config.Clusters["test"] = &api.Cluster{Server: "https://test"}
	config.AuthInfos["test"] = &api.AuthInfo{Token: "tok"}
	config.Contexts["test"] = &api.Context{Cluster: "test", AuthInfo: "test"}

	stampCfgctlExtensions(config, "aws-discovery")

	if !isCfgctlManaged(config.Clusters["test"].Extensions) {
		t.Error("cluster should be cfgctl-managed after stamping")
	}
	if !isCfgctlManaged(config.AuthInfos["test"].Extensions) {
		t.Error("authinfo should be cfgctl-managed after stamping")
	}
	if !isCfgctlManaged(config.Contexts["test"].Extensions) {
		t.Error("context should be cfgctl-managed after stamping")
	}
}

func TestStampCfgctlExtensionsNilConfig(_ *testing.T) {
	stampCfgctlExtensions(nil, "test")
}

func TestPreserveUnmanagedEntries(t *testing.T) {
	const dockerDesktop = "docker-desktop"

	existing := newKubeconfig()

	existing.Clusters[dockerDesktop] = &api.Cluster{Server: "https://docker.internal:6443"}
	existing.AuthInfos[dockerDesktop] = &api.AuthInfo{Token: "docker-tok"}
	existing.Contexts[dockerDesktop] = &api.Context{Cluster: dockerDesktop, AuthInfo: dockerDesktop}

	existing.Clusters["old-eks"] = &api.Cluster{Server: "https://old-eks.example.com"}
	existing.AuthInfos["old-eks"] = &api.AuthInfo{}
	existing.Contexts["old-eks"] = &api.Context{Cluster: "old-eks", AuthInfo: "old-eks"}
	stampCfgctlExtensions(existing, "aws-discovery")

	// Only stamp old-eks as managed; docker-desktop remains unmanaged.
	// Since stampCfgctlExtensions stamps everything, we need to remove it from docker-desktop.
	delete(existing.Clusters[dockerDesktop].Extensions, cfgctlExtensionKey)
	delete(existing.AuthInfos[dockerDesktop].Extensions, cfgctlExtensionKey)
	delete(existing.Contexts[dockerDesktop].Extensions, cfgctlExtensionKey)

	existing.CurrentContext = dockerDesktop

	target := newKubeconfig()
	preserveUnmanagedEntries(target, existing)

	if _, ok := target.Clusters[dockerDesktop]; !ok {
		t.Error("expected docker-desktop cluster to be preserved")
	}
	if _, ok := target.AuthInfos[dockerDesktop]; !ok {
		t.Error("expected docker-desktop authinfo to be preserved")
	}
	if _, ok := target.Contexts[dockerDesktop]; !ok {
		t.Error("expected docker-desktop context to be preserved")
	}

	if _, ok := target.Clusters["old-eks"]; ok {
		t.Error("cfgctl-managed cluster should NOT be preserved")
	}
	if _, ok := target.AuthInfos["old-eks"]; ok {
		t.Error("cfgctl-managed authinfo should NOT be preserved")
	}
	if _, ok := target.Contexts["old-eks"]; ok {
		t.Error("cfgctl-managed context should NOT be preserved")
	}

	if target.CurrentContext != dockerDesktop {
		t.Errorf("CurrentContext = %q, want %q", target.CurrentContext, dockerDesktop)
	}
}

func TestPreserveUnmanagedEntriesNoOverwrite(t *testing.T) {
	existing := newKubeconfig()
	existing.Clusters["shared"] = &api.Cluster{Server: "https://old"}
	existing.Contexts["shared"] = &api.Context{Cluster: "shared"}

	target := newKubeconfig()
	target.Clusters["shared"] = &api.Cluster{Server: "https://new"}
	target.Contexts["shared"] = &api.Context{Cluster: "shared-new"}

	preserveUnmanagedEntries(target, existing)

	if target.Clusters["shared"].Server != "https://new" {
		t.Error("existing entry should not overwrite target entry")
	}
	if target.Contexts["shared"].Cluster != "shared-new" {
		t.Error("existing context should not overwrite target context")
	}
}

func TestPreserveUnmanagedEntriesNils(_ *testing.T) {
	preserveUnmanagedEntries(nil, newKubeconfig())
	preserveUnmanagedEntries(newKubeconfig(), nil)
}

func TestPreserveUnmanagedCurrentContextManagedEntry(t *testing.T) {
	existing := newKubeconfig()
	existing.Contexts["managed-ctx"] = &api.Context{Cluster: "c", AuthInfo: "u"}
	stampCfgctlExtensions(existing, "aws-discovery")
	existing.CurrentContext = "managed-ctx"

	target := newKubeconfig()
	preserveUnmanagedEntries(target, existing)

	if target.CurrentContext != "" {
		t.Errorf("CurrentContext = %q, should be empty for managed context", target.CurrentContext)
	}
}

func TestBuildKubeconfigStampsExtensions(t *testing.T) {
	clusters := []DiscoveredCluster{
		{
			Profile:  "prod",
			Region:   "us-west-2",
			Name:     "app",
			Endpoint: "https://app.example.com",
			CAData:   []byte("ca"),
		},
	}

	config, err := BuildKubeconfig(clusters, defaultNamingPatternValue)
	if err != nil {
		t.Fatalf("BuildKubeconfig failed: %v", err)
	}

	name := "prod-app"
	if !isCfgctlManaged(config.Clusters[name].Extensions) {
		t.Error("discovered cluster should be cfgctl-managed")
	}
	if !isCfgctlManaged(config.Contexts[name].Extensions) {
		t.Error("discovered context should be cfgctl-managed")
	}
	if !isCfgctlManaged(config.AuthInfos[name].Extensions) {
		t.Error("discovered authinfo should be cfgctl-managed")
	}
}

func TestBuildManualKubeconfigStampsExtensions(t *testing.T) {
	manualConfigs := []ManualConfig{
		{
			Name:            "manual-cluster",
			ClusterEndpoint: "https://manual.example.com",
			AuthInfo:        ManualAuthInfo{Token: "tok"},
		},
	}

	config, err := buildManualKubeconfig(manualConfigs)
	if err != nil {
		t.Fatalf("buildManualKubeconfig failed: %v", err)
	}

	if !isCfgctlManaged(config.Clusters["manual-cluster"].Extensions) {
		t.Error("manual cluster should be cfgctl-managed")
	}
	if !isCfgctlManaged(config.Contexts["manual-cluster"].Extensions) {
		t.Error("manual context should be cfgctl-managed")
	}
}

func TestExtensionsSurviveWriteAndLoad(t *testing.T) {
	config := newKubeconfig()
	config.Clusters["test"] = &api.Cluster{Server: "https://test.example.com"}
	config.AuthInfos["test"] = &api.AuthInfo{Token: "tok"}
	config.Contexts["test"] = &api.Context{Cluster: "test", AuthInfo: "test"}
	stampCfgctlExtensions(config, "aws-discovery")

	dir := t.TempDir()
	path := filepath.Join(dir, "config")

	if err := clientcmd.WriteToFile(*config, path); err != nil {
		t.Fatalf("WriteToFile failed: %v", err)
	}

	loaded, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if !isCfgctlManaged(loaded.Contexts["test"].Extensions) {
		t.Error("extension should survive write/load round-trip on context")
	}
	if !isCfgctlManaged(loaded.Clusters["test"].Extensions) {
		t.Error("extension should survive write/load round-trip on cluster")
	}
	if !isCfgctlManaged(loaded.AuthInfos["test"].Extensions) {
		t.Error("extension should survive write/load round-trip on authinfo")
	}
}

func TestMergeKubeconfigsPreservesUnmanagedEntries(t *testing.T) {
	const dockerDesktopMerge = "docker-desktop"

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config")

	existing := newKubeconfig()

	existing.Clusters[dockerDesktopMerge] = &api.Cluster{Server: "https://docker.internal:6443"}
	existing.AuthInfos[dockerDesktopMerge] = &api.AuthInfo{Token: "docker-tok"}
	existing.Contexts[dockerDesktopMerge] = &api.Context{Cluster: dockerDesktopMerge, AuthInfo: dockerDesktopMerge}
	existing.CurrentContext = dockerDesktopMerge

	existing.Clusters["old-managed"] = &api.Cluster{Server: "https://old-managed"}
	existing.AuthInfos["old-managed"] = &api.AuthInfo{}
	existing.Contexts["old-managed"] = &api.Context{Cluster: "old-managed", AuthInfo: "old-managed"}
	stampCfgctlExtensions(existing, "aws-discovery")
	delete(existing.Clusters[dockerDesktopMerge].Extensions, cfgctlExtensionKey)
	delete(existing.AuthInfos[dockerDesktopMerge].Extensions, cfgctlExtensionKey)
	delete(existing.Contexts[dockerDesktopMerge].Extensions, cfgctlExtensionKey)

	if err := clientcmd.WriteToFile(*existing, outputPath); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	discovered := newKubeconfig()
	discovered.Clusters["new-eks"] = &api.Cluster{Server: "https://new-eks.example.com"}
	discovered.AuthInfos["new-eks"] = &api.AuthInfo{}
	discovered.Contexts["new-eks"] = &api.Context{Cluster: "new-eks", AuthInfo: "new-eks"}
	stampCfgctlExtensions(discovered, "aws-discovery")

	mergeDir := filepath.Join(dir, "merge")
	if err := os.MkdirAll(mergeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	mergeConfig := MergeConfig{
		SourceDir:       mergeDir,
		IncludePatterns: []string{"*.yaml"},
	}

	merged, _, err := MergeKubeconfigs(outputPath, mergeConfig, discovered)
	if err != nil {
		t.Fatalf("MergeKubeconfigs failed: %v", err)
	}

	if _, ok := merged.Clusters[dockerDesktopMerge]; !ok {
		t.Error("unmanaged docker-desktop cluster should be preserved")
	}
	if _, ok := merged.Contexts[dockerDesktopMerge]; !ok {
		t.Error("unmanaged docker-desktop context should be preserved")
	}

	if _, ok := merged.Clusters["old-managed"]; ok {
		t.Error("stale cfgctl-managed cluster should NOT be preserved")
	}

	if _, ok := merged.Clusters["new-eks"]; !ok {
		t.Error("newly discovered cluster should be present")
	}

	if merged.CurrentContext != dockerDesktopMerge {
		t.Errorf("CurrentContext = %q, want %q", merged.CurrentContext, dockerDesktopMerge)
	}
}

func TestNonMergePathPreservesUnmanagedEntries(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config")

	existing := newKubeconfig()
	existing.Clusters["minikube"] = &api.Cluster{Server: "https://192.168.49.2:8443"}
	existing.AuthInfos["minikube"] = &api.AuthInfo{Token: "minikube-tok"}
	existing.Contexts["minikube"] = &api.Context{Cluster: "minikube", AuthInfo: "minikube"}

	if err := clientcmd.WriteToFile(*existing, outputPath); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = outputPath
	cfg.MergeEnabled = false
	cfg.ManualConfigs = []ManualConfig{
		{
			Name:            "manual-entry",
			ClusterEndpoint: "https://manual.example.com",
			AuthInfo:        ManualAuthInfo{Token: "tok"},
		},
	}

	p := NewProvider(cfg)
	merged, _, err := p.buildKubeconfig(nil)
	if err != nil {
		t.Fatalf("buildKubeconfig failed: %v", err)
	}

	if _, ok := merged.Clusters["minikube"]; !ok {
		t.Error("unmanaged minikube cluster should be preserved in non-merge path")
	}
	if _, ok := merged.Contexts["minikube"]; !ok {
		t.Error("unmanaged minikube context should be preserved")
	}

	if _, ok := merged.Clusters["manual-entry"]; !ok {
		t.Error("manual entry should be present")
	}
}

func TestFirstRunTransitionPreservesAllEntries(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config")

	// Simulate a pre-upgrade kubeconfig with no extensions on any entry.
	existing := newKubeconfig()
	existing.Clusters["legacy-eks"] = &api.Cluster{Server: "https://legacy-eks.example.com"}
	existing.AuthInfos["legacy-eks"] = &api.AuthInfo{Token: "legacy-tok"}
	existing.Contexts["legacy-eks"] = &api.Context{Cluster: "legacy-eks", AuthInfo: "legacy-eks"}
	existing.Clusters["rancher"] = &api.Cluster{Server: "https://rancher.local:6443"}
	existing.AuthInfos["rancher"] = &api.AuthInfo{Token: "rancher-tok"}
	existing.Contexts["rancher"] = &api.Context{Cluster: "rancher", AuthInfo: "rancher"}
	existing.CurrentContext = "rancher"

	if err := clientcmd.WriteToFile(*existing, outputPath); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	discovered := newKubeconfig()
	discovered.Clusters["new-eks"] = &api.Cluster{Server: "https://new-eks.example.com"}
	discovered.AuthInfos["new-eks"] = &api.AuthInfo{}
	discovered.Contexts["new-eks"] = &api.Context{Cluster: "new-eks", AuthInfo: "new-eks"}
	stampCfgctlExtensions(discovered, "aws-discovery")

	mergeDir := filepath.Join(dir, "merge")
	if err := os.MkdirAll(mergeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	merged, _, err := MergeKubeconfigs(outputPath, MergeConfig{
		SourceDir:       mergeDir,
		IncludePatterns: []string{"*.yaml"},
	}, discovered)
	if err != nil {
		t.Fatalf("MergeKubeconfigs failed: %v", err)
	}

	if _, ok := merged.Clusters["legacy-eks"]; !ok {
		t.Error("legacy-eks should be preserved on first run (no extensions = unmanaged)")
	}
	if _, ok := merged.Clusters["rancher"]; !ok {
		t.Error("rancher should be preserved on first run (no extensions = unmanaged)")
	}
	if _, ok := merged.Clusters["new-eks"]; !ok {
		t.Error("newly discovered cluster should be present")
	}
	if merged.CurrentContext != "rancher" {
		t.Errorf("CurrentContext = %q, want %q", merged.CurrentContext, "rancher")
	}
}

func TestNameConflictDiscoveredWinsOverUnmanaged(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config")

	existing := newKubeconfig()
	existing.Clusters["prod-app"] = &api.Cluster{Server: "https://old-manual-server"}
	existing.AuthInfos["prod-app"] = &api.AuthInfo{Token: "old-tok"}
	existing.Contexts["prod-app"] = &api.Context{Cluster: "prod-app", AuthInfo: "prod-app"}

	if err := clientcmd.WriteToFile(*existing, outputPath); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	discovered := newKubeconfig()
	discovered.Clusters["prod-app"] = &api.Cluster{Server: "https://discovered-server"}
	discovered.AuthInfos["prod-app"] = &api.AuthInfo{}
	discovered.Contexts["prod-app"] = &api.Context{Cluster: "prod-app", AuthInfo: "prod-app"}
	stampCfgctlExtensions(discovered, "aws-discovery")

	mergeDir := filepath.Join(dir, "merge")
	if err := os.MkdirAll(mergeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	merged, _, err := MergeKubeconfigs(outputPath, MergeConfig{
		SourceDir:       mergeDir,
		IncludePatterns: []string{"*.yaml"},
	}, discovered)
	if err != nil {
		t.Fatalf("MergeKubeconfigs failed: %v", err)
	}

	cluster := merged.Clusters["prod-app"]
	if cluster == nil {
		t.Fatal("prod-app cluster missing")
	}
	if cluster.Server != "https://discovered-server" {
		t.Errorf("Server = %q, want discovered to win over unmanaged", cluster.Server)
	}
	if !isCfgctlManaged(cluster.Extensions) {
		t.Error("prod-app should now be cfgctl-managed after discovery overwrites")
	}
}

func TestNonMergePathDropsStaleManagedEntries(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config")

	existing := newKubeconfig()

	existing.Clusters["stale-eks"] = &api.Cluster{Server: "https://stale-eks.example.com"}
	existing.AuthInfos["stale-eks"] = &api.AuthInfo{}
	existing.Contexts["stale-eks"] = &api.Context{Cluster: "stale-eks", AuthInfo: "stale-eks"}
	stampCfgctlExtensions(existing, "aws-discovery")

	existing.Clusters["minikube"] = &api.Cluster{Server: "https://192.168.49.2:8443"}
	existing.AuthInfos["minikube"] = &api.AuthInfo{Token: "mk-tok"}
	existing.Contexts["minikube"] = &api.Context{Cluster: "minikube", AuthInfo: "minikube"}

	if err := clientcmd.WriteToFile(*existing, outputPath); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = outputPath
	cfg.MergeEnabled = false
	cfg.ManualConfigs = []ManualConfig{
		{
			Name:            "on-prem",
			ClusterEndpoint: "https://on-prem.example.com",
			AuthInfo:        ManualAuthInfo{Token: "tok"},
		},
	}

	// No discovered clusters — stale-eks was removed from AWS.
	p := NewProvider(cfg)
	merged, _, err := p.buildKubeconfig(nil)
	if err != nil {
		t.Fatalf("buildKubeconfig failed: %v", err)
	}

	if _, ok := merged.Clusters["stale-eks"]; ok {
		t.Error("stale cfgctl-managed entry should be dropped when cluster no longer discovered")
	}
	if _, ok := merged.Clusters["minikube"]; !ok {
		t.Error("unmanaged minikube should be preserved")
	}
	if _, ok := merged.Clusters["on-prem"]; !ok {
		t.Error("manual entry should be present")
	}
}

func TestMixedSourcesCoexist(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config")

	existing := newKubeconfig()
	existing.Clusters["docker-desktop"] = &api.Cluster{Server: "https://docker.internal:6443"}
	existing.AuthInfos["docker-desktop"] = &api.AuthInfo{Token: "docker-tok"}
	existing.Contexts["docker-desktop"] = &api.Context{Cluster: "docker-desktop", AuthInfo: "docker-desktop"}
	existing.CurrentContext = "docker-desktop"

	if err := clientcmd.WriteToFile(*existing, outputPath); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	discovered := []DiscoveredCluster{
		{
			Profile:  "prod",
			Region:   "us-west-2",
			Name:     "payments",
			Endpoint: "https://payments.example.com",
			CAData:   []byte("ca"),
		},
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = outputPath
	cfg.MergeEnabled = false
	cfg.NamingPattern = defaultNamingPatternValue
	cfg.ManualConfigs = []ManualConfig{
		{
			Name:            "on-prem",
			ClusterEndpoint: "https://on-prem.example.com",
			AuthInfo:        ManualAuthInfo{Token: "tok"},
		},
	}

	p := NewProvider(cfg)
	merged, _, err := p.buildKubeconfig(discovered)
	if err != nil {
		t.Fatalf("buildKubeconfig failed: %v", err)
	}

	if _, ok := merged.Clusters["docker-desktop"]; !ok {
		t.Error("unmanaged docker-desktop should be preserved")
	}
	if !isCfgctlManaged(merged.Clusters["prod-payments"].Extensions) {
		t.Error("discovered prod-payments should be cfgctl-managed")
	}
	if !isCfgctlManaged(merged.Clusters["on-prem"].Extensions) {
		t.Error("manual on-prem should be cfgctl-managed")
	}
	if isCfgctlManaged(merged.Clusters["docker-desktop"].Extensions) {
		t.Error("docker-desktop should NOT be cfgctl-managed")
	}

	if merged.CurrentContext != "docker-desktop" {
		t.Errorf("CurrentContext = %q, want %q", merged.CurrentContext, "docker-desktop")
	}

	if len(merged.Clusters) != 3 {
		t.Errorf("expected 3 clusters (unmanaged + discovered + manual), got %d", len(merged.Clusters))
	}
}
