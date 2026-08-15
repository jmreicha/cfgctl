package cli

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const testCtxName = "my-ctx"

func writeTestKubeconfig(t *testing.T, path string, config *api.Config) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := clientcmd.WriteToFile(*config, path); err != nil {
		t.Fatal(err)
	}
}

func testKubeconfig(contexts ...string) *api.Config {
	config := api.NewConfig()
	for _, name := range contexts {
		config.Clusters[name] = &api.Cluster{Server: "https://" + name + ".example.com"}
		config.AuthInfos[name] = &api.AuthInfo{Token: "token-" + name}
		config.Contexts[name] = &api.Context{Cluster: name, AuthInfo: name}
	}
	if len(contexts) > 0 {
		config.CurrentContext = contexts[0]
	}
	return config
}

// sharedRefKubeconfig builds a config where both contexts point at the same
// cluster and user, mirroring a kubeconfig with a single backing cluster.
func sharedRefKubeconfig(ctxA, ctxB, cluster, user string) *api.Config {
	config := api.NewConfig()
	config.Clusters[cluster] = &api.Cluster{Server: "https://" + cluster + ".example.com"}
	config.AuthInfos[user] = &api.AuthInfo{Token: "token-" + user}
	config.Contexts[ctxA] = &api.Context{Cluster: cluster, AuthInfo: user}
	config.Contexts[ctxB] = &api.Context{Cluster: cluster, AuthInfo: user}
	config.CurrentContext = ctxA
	return config
}

func TestKubeMerge(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	source := filepath.Join(dir, "source.yaml")

	writeTestKubeconfig(t, target, testKubeconfig("existing"))
	writeTestKubeconfig(t, source, testKubeconfig("new-ctx"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeMergeCmd()
	cmd.SetArgs([]string{source})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Contexts["existing"]; !ok {
		t.Fatal("expected existing context to be preserved")
	}
	if _, ok := result.Contexts["new-ctx"]; !ok {
		t.Fatal("expected new-ctx to be merged in")
	}
}

func TestKubeMerge_TargetMissing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config") // does not exist yet
	source := filepath.Join(dir, "source.yaml")
	writeTestKubeconfig(t, source, testKubeconfig("new-ctx"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeMergeCmd()
	cmd.SetArgs([]string{source})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("merge into missing target failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Contexts["new-ctx"]; !ok {
		t.Fatal("expected new-ctx in freshly created target")
	}
}

func TestKubeMerge_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	source1 := filepath.Join(dir, "s1.yaml")
	source2 := filepath.Join(dir, "s2.yaml")

	writeTestKubeconfig(t, target, testKubeconfig("base"))
	writeTestKubeconfig(t, source1, testKubeconfig("cluster-a"))
	writeTestKubeconfig(t, source2, testKubeconfig("cluster-b"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeMergeCmd()
	cmd.SetArgs([]string{source1, source2})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"base", "cluster-a", "cluster-b"} {
		if _, ok := result.Contexts[name]; !ok {
			t.Fatalf("expected context %q", name)
		}
	}
}

func TestKubeDelete(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	writeTestKubeconfig(t, target, testKubeconfig("keep", "remove"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeDeleteCmd()
	cmd.SetArgs([]string{"remove"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Contexts["keep"]; !ok {
		t.Fatal("expected keep context to remain")
	}
	if _, ok := result.Contexts["remove"]; ok {
		t.Fatal("expected remove context to be deleted")
	}
	if _, ok := result.Clusters["remove"]; ok {
		t.Fatal("expected orphaned cluster to be removed")
	}
	if _, ok := result.AuthInfos["remove"]; ok {
		t.Fatal("expected orphaned authinfo to be removed")
	}
}

func TestKubeDelete_Multiple(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	writeTestKubeconfig(t, target, testKubeconfig("a", "b", "c"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeDeleteCmd()
	cmd.SetArgs([]string{"a", "c"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Contexts["b"]; !ok {
		t.Fatal("expected context b to remain")
	}
	if len(result.Contexts) != 1 {
		t.Fatalf("expected 1 context, got %d", len(result.Contexts))
	}
}

func TestKubeDelete_NotFound(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	writeTestKubeconfig(t, target, testKubeconfig("exists"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeDeleteCmd()
	cmd.SetArgs([]string{"nope"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing context")
	}
}

func TestKubeDelete_ClearsCurrentContext(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	writeTestKubeconfig(t, target, testKubeconfig("current"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeDeleteCmd()
	cmd.SetArgs([]string{"current"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if result.CurrentContext != "" {
		t.Fatalf("expected current-context to be cleared, got %q", result.CurrentContext)
	}
}

func TestKubeRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	writeTestKubeconfig(t, target, testKubeconfig("old-name"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeRenameCmd()
	cmd.SetArgs([]string{"old-name", "new-name"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rename failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Contexts["old-name"]; ok {
		t.Fatal("expected old context to be removed")
	}
	if _, ok := result.Contexts["new-name"]; !ok {
		t.Fatal("expected new context to exist")
	}
	if _, ok := result.Clusters["new-name"]; !ok {
		t.Fatal("expected cluster to be renamed")
	}
	if _, ok := result.AuthInfos["new-name"]; !ok {
		t.Fatal("expected authinfo to be renamed")
	}
	if result.CurrentContext != "new-name" {
		t.Fatalf("expected current-context to be updated, got %q", result.CurrentContext)
	}
}

func TestKubeRename_TargetExists(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	writeTestKubeconfig(t, target, testKubeconfig("a", "b"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeRenameCmd()
	cmd.SetArgs([]string{"a", "b"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when target context exists")
	}
}

func TestKubeExport_Stdout(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "config")
	writeTestKubeconfig(t, source, testKubeconfig(testCtxName, "other"))

	kubeconfig = source
	defer func() { kubeconfig = "" }()

	cmd := newKubeExportCmd()
	cmd.SetArgs([]string{testCtxName})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("export failed: %v", err)
	}
}

func TestKubeExport_File(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "config")
	output := filepath.Join(dir, "exported.yaml")
	writeTestKubeconfig(t, source, testKubeconfig(testCtxName, "other"))

	kubeconfig = source
	defer func() { kubeconfig = "" }()

	cmd := newKubeExportCmd()
	cmd.SetArgs([]string{testCtxName, "--output", output})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contexts) != 1 {
		t.Fatalf("expected 1 context, got %d", len(result.Contexts))
	}
	if _, ok := result.Contexts[testCtxName]; !ok {
		t.Fatal("expected my-ctx context in export")
	}
	if _, ok := result.Contexts["other"]; ok {
		t.Fatal("expected other context to be excluded from export")
	}
	if result.CurrentContext != testCtxName {
		t.Fatalf("expected current-context=my-ctx, got %q", result.CurrentContext)
	}
}

func TestKubeExport_NotFound(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "config")
	writeTestKubeconfig(t, source, testKubeconfig("exists"))

	kubeconfig = source
	defer func() { kubeconfig = "" }()

	cmd := newKubeExportCmd()
	cmd.SetArgs([]string{"nope"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing context")
	}
}

func TestKubeMerge_Backup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	source := filepath.Join(dir, "source.yaml")

	writeTestKubeconfig(t, target, testKubeconfig("existing"))
	writeTestKubeconfig(t, source, testKubeconfig("new-ctx"))

	noBackup = false
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeMergeCmd()
	cmd.SetArgs([]string{source})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if _, err := os.Stat(target + ".bak"); err != nil {
		t.Fatal("expected backup file to be created")
	}
}

func TestResolveKubeconfig_Flag(t *testing.T) {
	kubeconfig = "/custom/path"
	defer func() { kubeconfig = "" }()

	if got := resolveKubeconfig(); got != "/custom/path" {
		t.Fatalf("expected /custom/path, got %q", got)
	}
}

func TestResolveKubeconfig_EnvVar(t *testing.T) {
	kubeconfig = ""
	t.Setenv("KUBECONFIG", "/from/env")

	if got := resolveKubeconfig(); got != "/from/env" {
		t.Fatalf("expected /from/env, got %q", got)
	}
}

func TestResolveKubeconfig_EnvVarPathList(t *testing.T) {
	kubeconfig = ""
	t.Setenv("KUBECONFIG", "/first/config:/second/config")

	if got := resolveKubeconfig(); got != "/first/config" {
		t.Fatalf("expected /first/config, got %q", got)
	}
}

func TestResolveKubeconfig_Default(t *testing.T) {
	kubeconfig = ""
	t.Setenv("KUBECONFIG", "")

	got := resolveKubeconfig()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".kube", "config")
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestKubeDelete_SharedClusterPreserved(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	writeTestKubeconfig(t, target, sharedRefKubeconfig("ctx-a", "ctx-b", "shared", "shared-user"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeDeleteCmd()
	cmd.SetArgs([]string{"ctx-a"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Contexts["ctx-a"]; ok {
		t.Fatal("expected ctx-a to be deleted")
	}
	if _, ok := result.Clusters["shared"]; !ok {
		t.Fatal("expected shared cluster to be preserved (still used by ctx-b)")
	}
	if _, ok := result.AuthInfos["shared-user"]; !ok {
		t.Fatal("expected shared user to be preserved (still used by ctx-b)")
	}
}

func TestKubeDelete_Atomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	writeTestKubeconfig(t, target, testKubeconfig("keep-me"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeDeleteCmd()
	cmd.SetArgs([]string{"keep-me", "does-not-exist"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing context in batch")
	}

	// The whole operation must abort before writing; keep-me survives on disk.
	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Contexts["keep-me"]; !ok {
		t.Fatal("expected keep-me to survive a failed batch delete")
	}
}

func TestKubeRename_SharedClusterUntouched(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	writeTestKubeconfig(t, target, sharedRefKubeconfig("ctx-a", "ctx-b", "shared", "shared-user"))

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeRenameCmd()
	cmd.SetArgs([]string{"ctx-a", "ctx-a-renamed"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rename failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	renamed, ok := result.Contexts["ctx-a-renamed"]
	if !ok {
		t.Fatal("expected renamed context")
	}
	// Shared cluster/user names must be left intact and still referenced.
	if renamed.Cluster != "shared" || renamed.AuthInfo != "shared-user" {
		t.Fatalf("expected renamed context to keep shared refs, got cluster=%q user=%q", renamed.Cluster, renamed.AuthInfo)
	}
	if _, ok := result.Clusters["shared"]; !ok {
		t.Fatal("expected shared cluster to remain")
	}
	if _, ok := result.Clusters["ctx-a-renamed"]; ok {
		t.Fatal("shared cluster must not be renamed to the new context name")
	}
}

func TestKubeRename_ClusterNameDiffersFromContext(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")

	// Context name differs from its cluster/user names (the EKS-merged shape).
	config := api.NewConfig()
	config.Clusters["eks-cluster"] = &api.Cluster{Server: "https://eks.example.com"}
	config.AuthInfos["eks-user"] = &api.AuthInfo{Token: "tok"}
	config.Contexts["prod"] = &api.Context{Cluster: "eks-cluster", AuthInfo: "eks-user"}
	config.CurrentContext = "prod"
	writeTestKubeconfig(t, target, config)

	noBackup = true
	defer func() { noBackup = false }()

	kubeconfig = target
	defer func() { kubeconfig = "" }()

	cmd := newKubeRenameCmd()
	cmd.SetArgs([]string{"prod", "production"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rename failed: %v", err)
	}

	result, err := clientcmd.LoadFromFile(target)
	if err != nil {
		t.Fatal(err)
	}
	renamed, ok := result.Contexts["production"]
	if !ok {
		t.Fatal("expected production context")
	}
	// Cluster/user names never matched the context name, so they must be untouched.
	if renamed.Cluster != "eks-cluster" || renamed.AuthInfo != "eks-user" {
		t.Fatalf("expected cluster/user refs unchanged, got cluster=%q user=%q", renamed.Cluster, renamed.AuthInfo)
	}
	if _, ok := result.Clusters["eks-cluster"]; !ok {
		t.Fatal("expected eks-cluster to remain")
	}
}
